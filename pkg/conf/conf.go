package conf

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/veesix-networks/osvbng/pkg/conf/handlers"
	"github.com/veesix-networks/osvbng/pkg/conf/types"
	"github.com/veesix-networks/osvbng/pkg/frr"
)

type ConfigDaemon struct {
	registry  *handlers.Registry
	frrConfig *frr.Config

	runningConfig *types.Config
	startupConfig *types.Config
	sessions      map[types.SessionID]*session
	versions      []types.ConfigVersion

	versionDir        string
	startupConfigPath string
	disableVersions   bool

	mu sync.RWMutex
}

type session struct {
	id      types.SessionID
	config  *types.Config
	changes []*handlers.HandlerContext
}

func NewConfigDaemon() *ConfigDaemon {
	return &ConfigDaemon{
		registry:          handlers.NewRegistry(),
		frrConfig:         frr.NewConfig(),
		runningConfig:     &types.Config{Interfaces: make(map[string]*types.InterfaceConfig)},
		startupConfig:     &types.Config{Interfaces: make(map[string]*types.InterfaceConfig)},
		sessions:          make(map[types.SessionID]*session),
		versions:          []types.ConfigVersion{},
		versionDir:        DefaultVersionDir,
		startupConfigPath: "/etc/osvbng/startup-config.yaml",
		disableVersions:   false,
	}
}

func (cd *ConfigDaemon) AutoRegisterHandlers(deps *handlers.ConfDeps) {
	cd.mu.Lock()
	defer cd.mu.Unlock()

	cd.registry.AutoRegisterAll(deps)
}

func (cd *ConfigDaemon) CreateCandidateSession() (types.SessionID, error) {
	cd.mu.Lock()
	defer cd.mu.Unlock()

	id := types.SessionID(fmt.Sprintf("session-%d", len(cd.sessions)+1))

	candidateConfig := &types.Config{
		Interfaces: make(map[string]*types.InterfaceConfig),
	}

	for k, v := range cd.runningConfig.Interfaces {
		ifCopy := *v
		candidateConfig.Interfaces[k] = &ifCopy
	}

	cd.sessions[id] = &session{
		id:     id,
		config: candidateConfig,
	}

	return id, nil
}

func (cd *ConfigDaemon) CloseCandidateSession(id types.SessionID) error {
	cd.mu.Lock()
	defer cd.mu.Unlock()

	if _, exists := cd.sessions[id]; !exists {
		return fmt.Errorf("session %s not found", id)
	}

	delete(cd.sessions, id)
	return nil
}

func (cd *ConfigDaemon) Set(id types.SessionID, path string, value interface{}) error {
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
		return fmt.Errorf("failed to get old value: %w", err)
	}

	hctx := &handlers.HandlerContext{
		SessionID: id,
		Path:      path,
		OldValue:  oldValue,
		NewValue:  value,
	}

	if err := handler.Validate(context.Background(), hctx); err != nil {
		return fmt.Errorf("validation failed: %w", err)
	}

	if err := setValueInConfig(sess.config, path, value); err != nil {
		return fmt.Errorf("failed to set value: %w", err)
	}

	sess.changes = append(sess.changes, hctx)

	return nil
}

func (cd *ConfigDaemon) Delete(id types.SessionID, path string) error {
	cd.mu.Lock()
	defer cd.mu.Unlock()

	sess, exists := cd.sessions[id]
	if !exists {
		return fmt.Errorf("session %s not found", id)
	}

	_ = sess

	return fmt.Errorf("Delete not yet implemented")
}

func (cd *ConfigDaemon) Modify(id types.SessionID, path string, value interface{}) error {
	cd.mu.Lock()
	defer cd.mu.Unlock()

	sess, exists := cd.sessions[id]
	if !exists {
		return fmt.Errorf("session %s not found", id)
	}

	_ = sess

	return fmt.Errorf("Modify not yet implemented")
}

func (cd *ConfigDaemon) Verify(id types.SessionID) ([]types.ValidationError, error) {
	cd.mu.RLock()
	defer cd.mu.RUnlock()

	sess, exists := cd.sessions[id]
	if !exists {
		return nil, fmt.Errorf("session %s not found", id)
	}

	var allErrors []types.ValidationError

	for _, change := range sess.changes {
		handler, err := cd.registry.GetHandler(change.Path)
		if err != nil {
			allErrors = append(allErrors, types.ValidationError{
				Path:    change.Path,
				Message: fmt.Sprintf("no handler for path: %v", err),
			})
			continue
		}

		if err := handler.Validate(context.Background(), change); err != nil {
			allErrors = append(allErrors, types.ValidationError{
				Path:    change.Path,
				Message: err.Error(),
			})
		}
	}

	return allErrors, nil
}

func (cd *ConfigDaemon) DryRun(id types.SessionID) (*types.DiffResult, error) {
	cd.mu.RLock()
	defer cd.mu.RUnlock()

	sess, exists := cd.sessions[id]
	if !exists {
		return nil, fmt.Errorf("session %s not found", id)
	}

	return FormatChanges(sess.changes), nil
}

func (cd *ConfigDaemon) Commit(id types.SessionID) error {
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

	appliedChanges := make([]*handlers.HandlerContext, 0)
	frrReloadNeeded := false
	for _, change := range sortedChanges {
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
		if err := cd.frrConfig.Test(sess.config); err != nil {
			cd.rollbackChanges(appliedChanges)
			return fmt.Errorf("FRR config validation failed: %w", err)
		}

		if err := cd.reloadFRR(sess.config); err != nil {
			cd.rollbackChanges(appliedChanges)
			return fmt.Errorf("FRR reload failed: %w", err)
		}
	}

	diff := FormatChanges(sess.changes)

	changes := make([]types.Change, 0)
	for _, line := range diff.Added {
		changes = append(changes, types.Change{Type: "add", Path: line.Path, Value: line.Value})
	}
	for _, line := range diff.Modified {
		changes = append(changes, types.Change{Type: "modify", Path: line.Path, Value: line.Value})
	}
	for _, line := range diff.Deleted {
		changes = append(changes, types.Change{Type: "delete", Path: line.Path, Value: line.Value})
	}

	version := types.ConfigVersion{
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

	return nil
}

func (cd *ConfigDaemon) reloadFRR(config *types.Config) error {
	return cd.frrConfig.Reload(config)
}

func (cd *ConfigDaemon) rollbackChanges(changes []*handlers.HandlerContext) {
	for i := len(changes) - 1; i >= 0; i-- {
		change := changes[i]
		handler, err := cd.registry.GetHandler(change.Path)
		if err != nil {
			continue
		}
		handler.Rollback(context.Background(), change)
	}
}

func (cd *ConfigDaemon) sortChangesByDependencies(changes []*handlers.HandlerContext) ([]*handlers.HandlerContext, error) {
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
			if graph[depPattern] == nil {
				graph[depPattern] = []int{}
			}
			graph[depPattern] = append(graph[depPattern], i)
			inDegree[i]++
		}
	}

	var sorted []*handlers.HandlerContext
	queue := make([]int, 0)

	for idx, degree := range inDegree {
		if degree == 0 {
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

func (cd *ConfigDaemon) Rollback(toVersion int) error {
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

	cd.sessions[sessionID].config = cd.deepCopyConfig(targetVersion.Config)

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
	appliedChanges := make([]*handlers.HandlerContext, 0)
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
	changes := make([]types.Change, 0)
	for _, line := range diff.Added {
		changes = append(changes, types.Change{Type: "add", Path: line.Path, Value: line.Value})
	}
	for _, line := range diff.Modified {
		changes = append(changes, types.Change{Type: "modify", Path: line.Path, Value: line.Value})
	}
	for _, line := range diff.Deleted {
		changes = append(changes, types.Change{Type: "delete", Path: line.Path, Value: line.Value})
	}

	version := types.ConfigVersion{
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

func (cd *ConfigDaemon) createSessionUnlocked() (types.SessionID, error) {
	id := types.SessionID(fmt.Sprintf("session-%d", len(cd.sessions)+1))

	candidateConfig := &types.Config{
		Interfaces: make(map[string]*types.InterfaceConfig),
	}

	for k, v := range cd.runningConfig.Interfaces {
		ifCopy := *v
		candidateConfig.Interfaces[k] = &ifCopy
	}

	cd.sessions[id] = &session{
		id:     id,
		config: candidateConfig,
	}

	return id, nil
}

func (cd *ConfigDaemon) verifyUnlocked(id types.SessionID) ([]types.ValidationError, error) {
	sess, exists := cd.sessions[id]
	if !exists {
		return nil, fmt.Errorf("session %s not found", id)
	}

	var allErrors []types.ValidationError

	for _, change := range sess.changes {
		handler, err := cd.registry.GetHandler(change.Path)
		if err != nil {
			allErrors = append(allErrors, types.ValidationError{
				Path:    change.Path,
				Message: fmt.Sprintf("no handler for path: %v", err),
			})
			continue
		}

		if err := handler.Validate(context.Background(), change); err != nil {
			allErrors = append(allErrors, types.ValidationError{
				Path:    change.Path,
				Message: err.Error(),
			})
		}
	}

	return allErrors, nil
}

func (cd *ConfigDaemon) ListVersions() ([]types.ConfigVersion, error) {
	cd.mu.RLock()
	defer cd.mu.RUnlock()

	return cd.versions, nil
}

func (cd *ConfigDaemon) GetRunning() (*types.Config, error) {
	cd.mu.RLock()
	defer cd.mu.RUnlock()

	return cd.runningConfig, nil
}

func (cd *ConfigDaemon) GetStartup() (*types.Config, error) {
	cd.mu.RLock()
	defer cd.mu.RUnlock()

	return cd.startupConfig, nil
}

func (cd *ConfigDaemon) SaveStartup() error {
	cd.mu.Lock()
	defer cd.mu.Unlock()

	cd.startupConfig = cd.deepCopyConfig(cd.runningConfig)
	return SaveYAML(cd.startupConfigPath, cd.startupConfig)
}

func (cd *ConfigDaemon) LoadConfig(id types.SessionID, config *types.Config) error {
	cd.mu.Lock()
	defer cd.mu.Unlock()

	sess, exists := cd.sessions[id]
	if !exists {
		return fmt.Errorf("session %s not found", id)
	}

	sess.changes = make([]*handlers.HandlerContext, 0)

	for name, iface := range config.Interfaces {
		hctx := &handlers.HandlerContext{
			SessionID: id,
			Path:      fmt.Sprintf("interfaces.%s", name),
			OldValue:  nil,
			NewValue:  iface,
		}
		sess.changes = append(sess.changes, hctx)

		if iface.Address != nil {
			for _, addr := range iface.Address.IPv4 {
				hctx := &handlers.HandlerContext{
					SessionID: id,
					Path:      fmt.Sprintf("interfaces.%s.address.ipv4", name),
					OldValue:  nil,
					NewValue:  addr,
				}
				sess.changes = append(sess.changes, hctx)
			}

			for _, addr := range iface.Address.IPv6 {
				hctx := &handlers.HandlerContext{
					SessionID: id,
					Path:      fmt.Sprintf("interfaces.%s.address.ipv6", name),
					OldValue:  nil,
					NewValue:  addr,
				}
				sess.changes = append(sess.changes, hctx)
			}
		}
	}

	if config.Protocols != nil {
		if config.Protocols.BGP != nil && config.Protocols.BGP.ASN != 0 {
			hctx := &handlers.HandlerContext{
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
				hctx := &handlers.HandlerContext{
					SessionID: id,
					Path:      fmt.Sprintf("protocols.static.ipv4.%d", i),
					OldValue:  nil,
					NewValue:  route,
				}
				sess.changes = append(sess.changes, hctx)
			}
			for i := range config.Protocols.Static.IPv6 {
				route := &config.Protocols.Static.IPv6[i]
				hctx := &handlers.HandlerContext{
					SessionID: id,
					Path:      fmt.Sprintf("protocols.static.ipv6.%d", i),
					OldValue:  nil,
					NewValue:  route,
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
