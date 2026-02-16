package configmgr

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/veesix-networks/osvbng/pkg/config"
	"github.com/veesix-networks/osvbng/pkg/config/interfaces"
	"github.com/veesix-networks/osvbng/pkg/deps"
	"github.com/veesix-networks/osvbng/pkg/frr"
	"github.com/veesix-networks/osvbng/pkg/handlers/conf"
	"github.com/veesix-networks/osvbng/pkg/handlers/conf/paths"
	"github.com/veesix-networks/osvbng/pkg/logger"
	pathspkg "github.com/veesix-networks/osvbng/pkg/paths"
	"github.com/veesix-networks/osvbng/pkg/southbound"
)

type ConfigManager struct {
	registry  *conf.Registry
	frrConfig *frr.Config
	logger    *slog.Logger

	runningConfig  *config.Config
	startupConfig  *config.Config
	dataplaneState *DataplaneState
	sessions       map[conf.SessionID]*session
	versions       []ConfigVersion

	versionDir        string
	startupConfigPath string
	disableVersions   bool

	mu sync.RWMutex
}

type session struct {
	id      conf.SessionID
	config  *config.Config
	changes []*conf.HandlerContext
}

func NewConfigManager() *ConfigManager {
	return &ConfigManager{
		registry:          conf.NewRegistry(),
		frrConfig:         frr.NewConfig(),
		logger:            logger.Get(logger.Config),
		runningConfig:     &config.Config{},
		startupConfig:     &config.Config{},
		sessions:          make(map[conf.SessionID]*session),
		versions:          []ConfigVersion{},
		versionDir:        DefaultVersionDir,
		startupConfigPath: "/etc/osvbng/startup-config.yaml",
		disableVersions:   false,
	}
}

func (cd *ConfigManager) AutoRegisterHandlers(deps *deps.ConfDeps) {
	cd.mu.Lock()
	defer cd.mu.Unlock()

	cd.registry.AutoRegisterAll(deps)
}

func (cd *ConfigManager) GetRegistry() *conf.Registry {
	return cd.registry
}

func (cd *ConfigManager) GetAllConfPaths() []paths.Path {
	cd.mu.RLock()
	defer cd.mu.RUnlock()

	return cd.registry.GetAllPaths()
}

func (cd *ConfigManager) CreateCandidateSession() (conf.SessionID, error) {
	cd.mu.Lock()
	defer cd.mu.Unlock()

	id := conf.SessionID(fmt.Sprintf("session-%d", len(cd.sessions)+1))

	candidateConfig := cd.deepCopyConfig(cd.runningConfig)

	cd.sessions[id] = &session{
		id:     id,
		config: candidateConfig,
	}

	return id, nil
}

func (cd *ConfigManager) CloseCandidateSession(id conf.SessionID) error {
	cd.mu.Lock()
	defer cd.mu.Unlock()

	if _, exists := cd.sessions[id]; !exists {
		return fmt.Errorf("session %s not found", id)
	}

	delete(cd.sessions, id)
	return nil
}

func (cd *ConfigManager) Set(id conf.SessionID, path string, value interface{}) error {
	cd.mu.Lock()
	defer cd.mu.Unlock()

	sess, exists := cd.sessions[id]
	if !exists {
		return fmt.Errorf("session %s not found", id)
	}

	handler, err := cd.registry.GetHandler(path)
	if err != nil {
		return fmt.Errorf("no handler for path %s: %w", path, err)
	}

	oldValue, err := getValueFromConfig(sess.config, path)
	if err != nil {
		oldValue = nil
	}

	hctx := &conf.HandlerContext{
		SessionID: id,
		Path:      path,
		OldValue:  oldValue,
		NewValue:  value,
	}

	if err := handler.Validate(context.Background(), hctx); err != nil {
		return fmt.Errorf("validation failed: %w", err)
	}

	if err := setValueInConfig(sess.config, path, value, handler.PathPattern()); err != nil {
		return fmt.Errorf("failed to set value: %w", err)
	}

	sess.changes = append(sess.changes, hctx)

	return nil
}

func (cd *ConfigManager) Delete(id conf.SessionID, path string) error {
	cd.mu.Lock()
	defer cd.mu.Unlock()

	sess, exists := cd.sessions[id]
	if !exists {
		return fmt.Errorf("session %s not found", id)
	}

	_ = sess

	return fmt.Errorf("Delete not yet implemented")
}

func (cd *ConfigManager) Modify(id conf.SessionID, path string, value interface{}) error {
	cd.mu.Lock()
	defer cd.mu.Unlock()

	sess, exists := cd.sessions[id]
	if !exists {
		return fmt.Errorf("session %s not found", id)
	}

	_ = sess

	return fmt.Errorf("Modify not yet implemented")
}

func (cd *ConfigManager) Verify(id conf.SessionID) ([]ValidationError, error) {
	cd.mu.RLock()
	defer cd.mu.RUnlock()

	sess, exists := cd.sessions[id]
	if !exists {
		return nil, fmt.Errorf("session %s not found", id)
	}

	var allErrors []ValidationError

	for _, change := range sess.changes {
		handler, err := cd.registry.GetHandler(change.Path)
		if err != nil {
			allErrors = append(allErrors, ValidationError{
				Path:    change.Path,
				Message: fmt.Sprintf("no handler for path: %v", err),
			})
			continue
		}

		if err := handler.Validate(context.Background(), change); err != nil {
			allErrors = append(allErrors, ValidationError{
				Path:    change.Path,
				Message: err.Error(),
			})
		}
	}

	return allErrors, nil
}

func (cd *ConfigManager) DryRun(id conf.SessionID) (*DiffResult, error) {
	cd.mu.RLock()
	defer cd.mu.RUnlock()

	sess, exists := cd.sessions[id]
	if !exists {
		return nil, fmt.Errorf("session %s not found", id)
	}

	return FormatChanges(sess.changes), nil
}

func (cd *ConfigManager) Commit(id conf.SessionID) error {
	cd.mu.Lock()
	defer cd.mu.Unlock()

	sess, exists := cd.sessions[id]
	if !exists {
		return fmt.Errorf("session %s not found", id)
	}

	sortedChanges, err := cd.sortChangesByDependencies(sess.changes)
	if err != nil {
		return fmt.Errorf("failed to resolve dependencies: %w", err)
	}

	if len(sortedChanges) == 0 {
		return fmt.Errorf("no changes to commit")
	}

	cd.logger.Info("Committing configuration", "session", id, "changes", len(sortedChanges))

	appliedChanges := make([]*conf.HandlerContext, 0)
	frrReloadNeeded := false
	for _, change := range sortedChanges {
		change.Config = sess.config
		handler, err := cd.registry.GetHandler(change.Path)
		if err != nil {
			cd.rollbackChanges(appliedChanges)
			return fmt.Errorf("no handler for path %s: %w", change.Path, err)
		}

		if err := cd.registry.ApplyWithCallbacks(context.Background(), handler, change); err != nil {
			cd.rollbackChanges(appliedChanges)
			return fmt.Errorf("failed to apply change to %s: %w", change.Path, err)
		}

		if change.IsFRRReloadNeeded() {
			frrReloadNeeded = true
		}

		appliedChanges = append(appliedChanges, change)
	}

	if frrReloadNeeded {
		cd.logger.Info("Reloading FRR configuration")
		if err := cd.frrConfig.Test(sess.config); err != nil {
			cd.rollbackChanges(appliedChanges)
			return fmt.Errorf("FRR config validation failed: %w", err)
		}

		if err := cd.reloadFRR(sess.config); err != nil {
			cd.rollbackChanges(appliedChanges)
			return fmt.Errorf("FRR reload failed: %w", err)
		}
		cd.logger.Info("FRR configuration reloaded successfully")
	}

	diff := FormatChanges(sess.changes)

	changes := make([]Change, 0)
	for _, line := range diff.Added {
		changes = append(changes, Change{Type: "add", Path: line.Path, Value: line.Value})
	}
	for _, line := range diff.Modified {
		changes = append(changes, Change{Type: "modify", Path: line.Path, Value: line.Value})
	}
	for _, line := range diff.Deleted {
		changes = append(changes, Change{Type: "delete", Path: line.Path, Value: line.Value})
	}

	version := ConfigVersion{
		Version:   len(cd.versions) + 1,
		Timestamp: time.Now(),
		Config:    nil,
		Changes:   changes,
	}

	cd.versions = append(cd.versions, version)
	cd.runningConfig = sess.config

	if !cd.disableVersions {
		if err := cd.saveVersion(version); err != nil {
			return fmt.Errorf("failed to save version: %w", err)
		}
	}

	cd.startupConfig = cd.deepCopyConfig(cd.runningConfig)
	if err := SaveYAML(cd.startupConfigPath, cd.startupConfig); err != nil {
		return fmt.Errorf("failed to save startup config: %w", err)
	}

	cd.logger.Info("Configuration committed successfully", "version", version.Version, "added", len(diff.Added), "modified", len(diff.Modified), "deleted", len(diff.Deleted))

	return nil
}

func (cd *ConfigManager) reloadFRR(config *config.Config) error {
	return cd.frrConfig.Reload(config)
}

func (cd *ConfigManager) rollbackChanges(changes []*conf.HandlerContext) {
	for i := len(changes) - 1; i >= 0; i-- {
		change := changes[i]
		handler, err := cd.registry.GetHandler(change.Path)
		if err != nil {
			continue
		}
		handler.Rollback(context.Background(), change)
	}
}

func (cd *ConfigManager) resolveDepPath(currentPath, currentPattern, depPattern string) (string, error) {
	if !strings.Contains(depPattern, "<") || !strings.Contains(depPattern, ">") {
		return depPattern, nil
	}

	wildcardValues, err := pathspkg.Extract(currentPath, currentPattern)
	if err != nil {
		return depPattern, nil
	}

	depWildcardCount := strings.Count(depPattern, "<")
	if depWildcardCount < len(wildcardValues) {
		wildcardValues = wildcardValues[:depWildcardCount]
	}

	resolvedPath, err := pathspkg.Build(depPattern, wildcardValues...)
	if err != nil {
		return depPattern, nil
	}

	return resolvedPath, nil
}

func (cd *ConfigManager) sortChangesByDependencies(changes []*conf.HandlerContext) ([]*conf.HandlerContext, error) {
	if len(changes) == 0 {
		return changes, nil
	}

	graph := make(map[string][]int)
	inDegree := make(map[int]int)
	patternToIndices := make(map[string][]int)

	for i, change := range changes {
		handler, err := cd.registry.GetHandler(change.Path)
		if err != nil {
			return nil, fmt.Errorf("no handler for path %s: %w", change.Path, err)
		}

		pathPattern := handler.PathPattern().String()
		patternToIndices[pathPattern] = append(patternToIndices[pathPattern], i)

		if _, exists := inDegree[i]; !exists {
			inDegree[i] = 0
		}

		for _, dep := range handler.Dependencies() {
			depPattern := dep.String()

			if _, existsInSession := patternToIndices[depPattern]; existsInSession {
				if graph[depPattern] == nil {
					graph[depPattern] = []int{}
				}
				graph[depPattern] = append(graph[depPattern], i)
				inDegree[i]++
			} else {
				resolvedDepPath, err := cd.resolveDepPath(change.Path, pathPattern, depPattern)
				if err != nil {
					return nil, err
				}

				val, err := getValueFromConfig(cd.runningConfig, resolvedDepPath)
				if err != nil || val == nil {
					userFriendlyDep := strings.ReplaceAll(resolvedDepPath, "<", "")
					userFriendlyDep = strings.ReplaceAll(userFriendlyDep, ">", "")
					return nil, fmt.Errorf("configuration '%s' requires '%s' to be configured first", change.Path, userFriendlyDep)
				}
			}
		}
	}

	var sorted []*conf.HandlerContext
	queue := make([]int, 0)

	// Collect and sort indices to preserve insertion order for changes at the
	// same dependency level. This ensures deterministic ordering — e.g., VRFs
	// (emitted first in LoadConfig) are processed before interfaces.
	indices := make([]int, 0, len(inDegree))
	for idx := range inDegree {
		indices = append(indices, idx)
	}
	sort.Ints(indices)
	for _, idx := range indices {
		if inDegree[idx] == 0 {
			queue = append(queue, idx)
		}
	}

	for len(queue) > 0 {
		currentIdx := queue[0]
		queue = queue[1:]

		sorted = append(sorted, changes[currentIdx])

		handler, _ := cd.registry.GetHandler(changes[currentIdx].Path)
		currentPattern := handler.PathPattern().String()

		for _, dependentIdx := range graph[currentPattern] {
			inDegree[dependentIdx]--
			if inDegree[dependentIdx] == 0 {
				queue = append(queue, dependentIdx)
			}
		}
	}

	if len(sorted) != len(changes) {
		return nil, fmt.Errorf("circular dependency detected in configuration changes")
	}

	return sorted, nil
}

func (cd *ConfigManager) Rollback(toVersion int) error {
	cd.mu.Lock()
	defer cd.mu.Unlock()

	if toVersion < 1 || toVersion > len(cd.versions) {
		return fmt.Errorf("invalid version: %d", toVersion)
	}

	targetVersion := cd.versions[toVersion-1]

	sessionID, err := cd.createSessionUnlocked()
	if err != nil {
		return fmt.Errorf("failed to create rollback session: %w", err)
	}

	versionConfig, ok := targetVersion.Config.(*config.Config)
	if !ok {
		delete(cd.sessions, sessionID)
		return fmt.Errorf("invalid config type in version %d", toVersion)
	}

	cd.sessions[sessionID].config = cd.deepCopyConfig(versionConfig)

	verifyErrs, err := cd.verifyUnlocked(sessionID)
	if err != nil {
		delete(cd.sessions, sessionID)
		return fmt.Errorf("rollback verification failed: %w", err)
	}
	if len(verifyErrs) > 0 {
		delete(cd.sessions, sessionID)
		return fmt.Errorf("rollback verification failed with %d errors", len(verifyErrs))
	}

	sess := cd.sessions[sessionID]
	appliedChanges := make([]*conf.HandlerContext, 0)
	for _, change := range sess.changes {
		handler, err := cd.registry.GetHandler(change.Path)
		if err != nil {
			cd.rollbackChanges(appliedChanges)
			delete(cd.sessions, sessionID)
			return fmt.Errorf("no handler for path %s: %w", change.Path, err)
		}

		if err := handler.Apply(context.Background(), change); err != nil {
			cd.rollbackChanges(appliedChanges)
			delete(cd.sessions, sessionID)
			return fmt.Errorf("failed to apply rollback to %s: %w", change.Path, err)
		}

		appliedChanges = append(appliedChanges, change)
	}

	cd.runningConfig = cd.sessions[sessionID].config

	diff := FormatChanges(sess.changes)
	changes := make([]Change, 0)
	for _, line := range diff.Added {
		changes = append(changes, Change{Type: "add", Path: line.Path, Value: line.Value})
	}
	for _, line := range diff.Modified {
		changes = append(changes, Change{Type: "modify", Path: line.Path, Value: line.Value})
	}
	for _, line := range diff.Deleted {
		changes = append(changes, Change{Type: "delete", Path: line.Path, Value: line.Value})
	}

	version := ConfigVersion{
		Version:   len(cd.versions) + 1,
		Timestamp: time.Now(),
		Config:    nil,
		Changes:   changes,
		CommitMsg: fmt.Sprintf("Rollback to version %d", toVersion),
	}

	cd.versions = append(cd.versions, version)

	delete(cd.sessions, sessionID)

	if !cd.disableVersions {
		if err := cd.saveVersion(version); err != nil {
			return fmt.Errorf("failed to save rollback version: %w", err)
		}
	}

	return nil
}

func (cd *ConfigManager) createSessionUnlocked() (conf.SessionID, error) {
	id := conf.SessionID(fmt.Sprintf("session-%d", len(cd.sessions)+1))

	candidateConfig := &config.Config{
		Interfaces: make(map[string]*interfaces.InterfaceConfig),
		Plugins:    make(map[string]interface{}),
	}

	for k, v := range cd.runningConfig.Interfaces {
		ifCopy := *v
		candidateConfig.Interfaces[k] = &ifCopy
	}

	for k, v := range cd.runningConfig.Plugins {
		candidateConfig.Plugins[k] = v
	}

	cd.sessions[id] = &session{
		id:     id,
		config: candidateConfig,
	}

	return id, nil
}

func (cd *ConfigManager) verifyUnlocked(id conf.SessionID) ([]ValidationError, error) {
	sess, exists := cd.sessions[id]
	if !exists {
		return nil, fmt.Errorf("session %s not found", id)
	}

	var allErrors []ValidationError

	for _, change := range sess.changes {
		handler, err := cd.registry.GetHandler(change.Path)
		if err != nil {
			allErrors = append(allErrors, ValidationError{
				Path:    change.Path,
				Message: fmt.Sprintf("no handler for path: %v", err),
			})
			continue
		}

		if err := handler.Validate(context.Background(), change); err != nil {
			allErrors = append(allErrors, ValidationError{
				Path:    change.Path,
				Message: err.Error(),
			})
		}
	}

	return allErrors, nil
}

func (cd *ConfigManager) ListVersions() ([]ConfigVersion, error) {
	cd.mu.RLock()
	defer cd.mu.RUnlock()

	return cd.versions, nil
}

func (cd *ConfigManager) GetRunning() (*config.Config, error) {
	cd.mu.RLock()
	defer cd.mu.RUnlock()

	return cd.runningConfig, nil
}

func (cd *ConfigManager) GetStartup() (*config.Config, error) {
	cd.mu.RLock()
	defer cd.mu.RUnlock()

	return cd.startupConfig, nil
}

func (cd *ConfigManager) LoadFromDataplane(sb southbound.Southbound) error {
	cd.mu.Lock()
	defer cd.mu.Unlock()

	cd.dataplaneState = NewDataplaneState()
	if err := cd.dataplaneState.LoadFromDataplane(sb); err != nil {
		return fmt.Errorf("load dataplane state: %w", err)
	}

	cd.logger.Info("Loaded dataplane state",
		"interfaces", len(cd.dataplaneState.Interfaces),
		"unnumbered", len(cd.dataplaneState.Unnumbered),
		"ipv6_enabled", len(cd.dataplaneState.IPv6Enabled),
		"punt_registrations", len(cd.dataplaneState.PuntRegistrations),
	)

	return nil
}

func (cd *ConfigManager) GetDataplaneState() *DataplaneState {
	cd.mu.RLock()
	defer cd.mu.RUnlock()

	return cd.dataplaneState
}

func (cd *ConfigManager) SaveStartup() error {
	cd.mu.Lock()
	defer cd.mu.Unlock()

	cd.startupConfig = cd.deepCopyConfig(cd.runningConfig)
	return SaveYAML(cd.startupConfigPath, cd.startupConfig)
}

func (cd *ConfigManager) ReloadFRR() error {
	cd.mu.RLock()
	defer cd.mu.RUnlock()

	return cd.reloadFRR(cd.runningConfig)
}

func (cd *ConfigManager) LoadConfig(id conf.SessionID, config *config.Config) error {
	cd.mu.Lock()
	defer cd.mu.Unlock()

	sess, exists := cd.sessions[id]
	if !exists {
		return fmt.Errorf("session %s not found", id)
	}

	sess.changes = make([]*conf.HandlerContext, 0)

	for name, vrfCfg := range config.VRFS {
		hctx := &conf.HandlerContext{
			SessionID: id,
			Path:      fmt.Sprintf("vrfs.%s", name),
			OldValue:  nil,
			NewValue:  vrfCfg,
		}
		sess.changes = append(sess.changes, hctx)
	}

	for name, sgCfg := range config.ServiceGroups {
		hctx := &conf.HandlerContext{
			SessionID: id,
			Path:      fmt.Sprintf("service-groups.%s", name),
			OldValue:  nil,
			NewValue:  sgCfg,
		}
		sess.changes = append(sess.changes, hctx)
	}

	for name, iface := range config.Interfaces {
		hctx := &conf.HandlerContext{
			SessionID: id,
			Path:      fmt.Sprintf("interfaces.%s", name),
			OldValue:  nil,
			NewValue:  iface,
		}
		sess.changes = append(sess.changes, hctx)

		if iface.Address != nil {
			for _, addr := range iface.Address.IPv4 {
				hctx := &conf.HandlerContext{
					SessionID: id,
					Path:      fmt.Sprintf("interfaces.%s.address.ipv4", name),
					OldValue:  nil,
					NewValue:  addr,
				}
				sess.changes = append(sess.changes, hctx)
			}

			for _, addr := range iface.Address.IPv6 {
				hctx := &conf.HandlerContext{
					SessionID: id,
					Path:      fmt.Sprintf("interfaces.%s.address.ipv6", name),
					OldValue:  nil,
					NewValue:  addr,
				}
				sess.changes = append(sess.changes, hctx)
			}
		}
	}

	if config.Protocols.BGP != nil && config.Protocols.BGP.ASN != 0 {
		hctx := &conf.HandlerContext{
			SessionID: id,
			Path:      "protocols.bgp.asn",
			OldValue:  nil,
			NewValue:  config.Protocols.BGP.ASN,
		}
		sess.changes = append(sess.changes, hctx)
	}

	if config.Protocols.Static != nil {
		for i := range config.Protocols.Static.IPv4 {
			route := &config.Protocols.Static.IPv4[i]
			hctx := &conf.HandlerContext{
				SessionID: id,
				Path:      fmt.Sprintf("protocols.static.ipv4.%d", i),
				OldValue:  nil,
				NewValue:  route,
			}
			sess.changes = append(sess.changes, hctx)
		}
		for i := range config.Protocols.Static.IPv6 {
			route := &config.Protocols.Static.IPv6[i]
			hctx := &conf.HandlerContext{
				SessionID: id,
				Path:      fmt.Sprintf("protocols.static.ipv6.%d", i),
				OldValue:  nil,
				NewValue:  route,
			}
			sess.changes = append(sess.changes, hctx)
		}
	}

	if config.Protocols.OSPF != nil && config.Protocols.OSPF.Enabled {
		hctx := &conf.HandlerContext{
			SessionID: id,
			Path:      "protocols.ospf.enabled",
			OldValue:  nil,
			NewValue:  true,
		}
		sess.changes = append(sess.changes, hctx)

		for areaID, area := range config.Protocols.OSPF.Areas {
			for ifName, ifCfg := range area.Interfaces {
				path, err := pathspkg.Build("protocols.ospf.areas.<*>.interfaces.<*>", areaID, ifName)
				if err != nil {
					return fmt.Errorf("build OSPF area interface path: %w", err)
				}
				hctx := &conf.HandlerContext{
					SessionID: id,
					Path:      path,
					OldValue:  nil,
					NewValue:  ifCfg,
				}
				sess.changes = append(sess.changes, hctx)
			}
		}
	}

	if config.Protocols.OSPF6 != nil && config.Protocols.OSPF6.Enabled {
		hctx := &conf.HandlerContext{
			SessionID: id,
			Path:      "protocols.ospf6.enabled",
			OldValue:  nil,
			NewValue:  true,
		}
		sess.changes = append(sess.changes, hctx)

		for areaID, area := range config.Protocols.OSPF6.Areas {
			for ifName, ifCfg := range area.Interfaces {
				path, err := pathspkg.Build("protocols.ospf6.areas.<*>.interfaces.<*>", areaID, ifName)
				if err != nil {
					return fmt.Errorf("build OSPFv3 area interface path: %w", err)
				}
				hctx := &conf.HandlerContext{
					SessionID: id,
					Path:      path,
					OldValue:  nil,
					NewValue:  ifCfg,
				}
				sess.changes = append(sess.changes, hctx)
			}
		}
	}

	if config.Protocols.MPLS != nil && config.Protocols.MPLS.Enabled {
		hctx := &conf.HandlerContext{
			SessionID: id,
			Path:      "protocols.mpls.enabled",
			OldValue:  nil,
			NewValue:  true,
		}
		sess.changes = append(sess.changes, hctx)

		if config.Protocols.MPLS.PlatformLabels > 0 {
			hctx := &conf.HandlerContext{
				SessionID: id,
				Path:      "protocols.mpls.platform-labels",
				OldValue:  nil,
				NewValue:  config.Protocols.MPLS.PlatformLabels,
			}
			sess.changes = append(sess.changes, hctx)
		}
	}

	if config.Protocols.LDP != nil && config.Protocols.LDP.Enabled {
		hctx := &conf.HandlerContext{
			SessionID: id,
			Path:      "protocols.ldp.enabled",
			OldValue:  nil,
			NewValue:  true,
		}
		sess.changes = append(sess.changes, hctx)

		if config.Protocols.LDP.RouterID != "" {
			hctx := &conf.HandlerContext{
				SessionID: id,
				Path:      "protocols.ldp.router-id",
				OldValue:  nil,
				NewValue:  config.Protocols.LDP.RouterID,
			}
			sess.changes = append(sess.changes, hctx)
		}

		if config.Protocols.LDP.AddressFamilies != nil {
			if config.Protocols.LDP.AddressFamilies.IPv4 != nil {
				hctx := &conf.HandlerContext{
					SessionID: id,
					Path:      "protocols.ldp.address-families.ipv4",
					OldValue:  nil,
					NewValue:  config.Protocols.LDP.AddressFamilies.IPv4,
				}
				sess.changes = append(sess.changes, hctx)
			}
			if config.Protocols.LDP.AddressFamilies.IPv6 != nil {
				hctx := &conf.HandlerContext{
					SessionID: id,
					Path:      "protocols.ldp.address-families.ipv6",
					OldValue:  nil,
					NewValue:  config.Protocols.LDP.AddressFamilies.IPv6,
				}
				sess.changes = append(sess.changes, hctx)
			}
		}

		for addr, neighborCfg := range config.Protocols.LDP.Neighbors {
			hctx := &conf.HandlerContext{
				SessionID: id,
				Path:      fmt.Sprintf("protocols.ldp.neighbors.%s", addr),
				OldValue:  nil,
				NewValue:  neighborCfg,
			}
			sess.changes = append(sess.changes, hctx)
		}
	}

	if config.System != nil && config.System.CPPM != nil {
		if config.System.CPPM.Dataplane != nil {
			for proto, policer := range config.System.CPPM.Dataplane.Policer {
				hctx := &conf.HandlerContext{
					SessionID: id,
					Path:      fmt.Sprintf("system.cppm.dataplane.policer.%s", proto),
					OldValue:  nil,
					NewValue:  policer,
				}
				sess.changes = append(sess.changes, hctx)
			}
		}
		if config.System.CPPM.Controlplane != nil {
			for proto, policer := range config.System.CPPM.Controlplane.Policer {
				hctx := &conf.HandlerContext{
					SessionID: id,
					Path:      fmt.Sprintf("system.cppm.controlplane.policer.%s", proto),
					OldValue:  nil,
					NewValue:  policer,
				}
				sess.changes = append(sess.changes, hctx)
			}
		}
	}

	for _, change := range sess.changes {
		handler, err := cd.registry.GetHandler(change.Path)
		if err != nil {
			continue
		}

		if err := handler.Validate(context.Background(), change); err != nil {
			return fmt.Errorf("validation failed for %s: %w", change.Path, err)
		}
	}

	sess.config = config
	return nil
}
