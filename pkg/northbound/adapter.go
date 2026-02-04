package northbound

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sort"
	"strings"

	"github.com/veesix-networks/osvbng/pkg/config"
	"github.com/veesix-networks/osvbng/pkg/configmgr"
	"github.com/veesix-networks/osvbng/pkg/handlers/conf"
	confpaths "github.com/veesix-networks/osvbng/pkg/handlers/conf/paths"
	"github.com/veesix-networks/osvbng/pkg/handlers/oper"
	operpaths "github.com/veesix-networks/osvbng/pkg/handlers/oper/paths"
	"github.com/veesix-networks/osvbng/pkg/handlers/show"
	showpaths "github.com/veesix-networks/osvbng/pkg/handlers/show/paths"
	"github.com/veesix-networks/osvbng/pkg/logger"
	"github.com/veesix-networks/osvbng/pkg/paths"
)

type Adapter struct {
	logger       *slog.Logger
	showRegistry *show.Registry
	confRegistry *conf.Registry
	operRegistry *oper.Registry
	configMgr    *configmgr.ConfigManager
}

func NewAdapter(showReg *show.Registry, confReg *conf.Registry, operReg *oper.Registry, configMgr *configmgr.ConfigManager) *Adapter {
	return &Adapter{
		logger:       logger.Component(logger.ComponentNorthbound),
		showRegistry: showReg,
		confRegistry: confReg,
		operRegistry: operReg,
		configMgr:    configMgr,
	}
}

func (a *Adapter) GetAllShowPaths() []showpaths.Path {
	return a.showRegistry.GetAllPaths()
}

func (a *Adapter) GetAllConfPaths() []confpaths.Path {
	return a.confRegistry.GetAllPaths()
}

func (a *Adapter) GetAllOperPaths() []operpaths.Path {
	return a.operRegistry.GetAllPaths()
}

func (a *Adapter) ExecuteOper(ctx context.Context, path string, body []byte, options map[string]string) (interface{}, error) {
	a.logger.Debug("Executing oper", "path", path)

	handler, err := a.operRegistry.GetHandler(path)
	if err != nil {
		a.logger.Error("Oper handler not found", "path", path, "error", err)
		return nil, fmt.Errorf("oper handler not found for path %s: %w", path, err)
	}

	req := &oper.Request{
		Path:    path,
		Body:    body,
		Options: options,
	}

	result, err := handler.Execute(ctx, req)
	if err != nil {
		a.logger.Error("Oper execution failed", "path", path, "error", err)
		return nil, err
	}

	return result, nil
}

func (a *Adapter) ExecuteShow(ctx context.Context, path string, options map[string]string) (interface{}, error) {
	a.logger.Debug("Executing show", "path", path)

	normalizedPath, err := a.NormalizePath(path, a.showRegistry.GetAllPaths())
	if err != nil {
		a.logger.Error("Failed to normalize show path", "path", path, "error", err)
		return nil, fmt.Errorf("failed to normalize show path: %w", err)
	}

	handler, err := a.showRegistry.GetHandler(normalizedPath)
	if err != nil {
		a.logger.Error("Show handler not found", "path", path, "normalized_path", normalizedPath, "error", err)
		return nil, fmt.Errorf("show handler not found for path %s: %w", path, err)
	}

	req := &show.Request{
		Path:    normalizedPath,
		Options: options,
	}

	result, err := handler.Collect(ctx, req)
	if err != nil {
		a.logger.Error("Show collection failed", "path", normalizedPath, "error", err)
		return nil, err
	}

	return result, nil
}

func (a *Adapter) ValidateConfig(ctx context.Context, sessionID conf.SessionID, path string, value interface{}) error {
	a.logger.Debug("Validating config", "session_id", sessionID, "path", path)

	handler, err := a.confRegistry.GetHandler(path)
	if err != nil {
		a.logger.Error("Config handler not found", "path", path, "error", err)
		return fmt.Errorf("config handler not found for path %s: %w", path, err)
	}

	hctx := &conf.HandlerContext{
		SessionID: sessionID,
		Path:      path,
		NewValue:  value,
	}

	if err := a.confRegistry.ValidateWithCallbacks(ctx, handler, hctx); err != nil {
		a.logger.Error("Config validation failed", "session_id", sessionID, "path", path, "error", err)
		return err
	}

	return nil
}

func (a *Adapter) ApplyConfig(ctx context.Context, sessionID conf.SessionID, path string, oldValue, newValue interface{}) error {
	a.logger.Debug("Applying config", "session_id", sessionID, "path", path)

	handler, err := a.confRegistry.GetHandler(path)
	if err != nil {
		a.logger.Error("Config handler not found", "path", path, "error", err)
		return fmt.Errorf("config handler not found for path %s: %w", path, err)
	}

	hctx := &conf.HandlerContext{
		SessionID: sessionID,
		Path:      path,
		OldValue:  oldValue,
		NewValue:  newValue,
	}

	if err := a.confRegistry.ApplyWithCallbacks(ctx, handler, hctx); err != nil {
		a.logger.Error("Config apply failed", "session_id", sessionID, "path", path, "error", err)
		return err
	}

	if hctx.IsFRRReloadNeeded() {
		a.logger.Info("Reloading FRR", "session_id", sessionID, "path", path)
		if err := a.configMgr.ReloadFRR(); err != nil {
			a.logger.Error("FRR reload failed", "session_id", sessionID, "error", err)
			return fmt.Errorf("FRR reload failed: %w", err)
		}
	}

	a.logger.Info("Config applied", "session_id", sessionID, "path", path)
	return nil
}

func (a *Adapter) RollbackConfig(ctx context.Context, sessionID conf.SessionID, path string, oldValue, newValue interface{}) error {
	a.logger.Warn("Rolling back config", "session_id", sessionID, "path", path)

	handler, err := a.confRegistry.GetHandler(path)
	if err != nil {
		a.logger.Error("Config handler not found", "path", path, "error", err)
		return fmt.Errorf("config handler not found for path %s: %w", path, err)
	}

	hctx := &conf.HandlerContext{
		SessionID: sessionID,
		Path:      path,
		OldValue:  oldValue,
		NewValue:  newValue,
	}

	if err := a.confRegistry.RollbackWithCallbacks(ctx, handler, hctx); err != nil {
		a.logger.Error("Config rollback failed", "session_id", sessionID, "path", path, "error", err)
		return err
	}

	a.logger.Info("Config rolled back", "session_id", sessionID, "path", path)
	return nil
}

func (a *Adapter) HasOperHandler(path string) bool {
	_, err := a.operRegistry.GetHandler(path)
	return err == nil
}

func (a *Adapter) GetRunningConfig(ctx context.Context) (*config.Config, error) {
	return a.configMgr.GetRunning()
}

func (a *Adapter) GetStartupConfig(ctx context.Context) (*config.Config, error) {
	return a.configMgr.GetStartup()
}

func (a *Adapter) SetAndCommit(ctx context.Context, path string, value interface{}) error {
	a.logger.Debug("SetAndCommit starting", "path", path)

	sessionID, err := a.configMgr.CreateCandidateSession()
	if err != nil {
		a.logger.Error("Failed to create candidate session", "error", err)
		return fmt.Errorf("failed to create candidate session: %w", err)
	}

	pathValues, err := a.flattenValue(path, value)
	if err != nil {
		a.logger.Error("Failed to flatten value", "path", path, "error", err)
		return fmt.Errorf("failed to flatten value: %w", err)
	}

	for _, pv := range pathValues {
		if err := a.configMgr.Set(sessionID, pv.path, pv.value); err != nil {
			a.logger.Error("Failed to set config value", "session_id", sessionID, "path", pv.path, "error", err)
			return fmt.Errorf("failed to set %s: %w", pv.path, err)
		}
	}

	if err := a.configMgr.Commit(sessionID); err != nil {
		a.logger.Error("Failed to commit", "session_id", sessionID, "error", err)
		return fmt.Errorf("failed to commit: %w", err)
	}

	a.logger.Info("SetAndCommit completed", "session_id", sessionID, "path", path)
	return nil
}

type pathValue struct {
	path  string
	value interface{}
}

func (a *Adapter) flattenValue(basePath string, value interface{}) ([]pathValue, error) {
	result := []pathValue{}

	if obj, ok := value.(map[string]interface{}); ok {
		for key, val := range obj {
			childPath := basePath + "." + key
			children, err := a.flattenValue(childPath, val)
			if err != nil {
				return nil, err
			}
			result = append(result, children...)
		}
		return result, nil
	}

	result = append(result, pathValue{path: basePath, value: value})
	return result, nil
}

func (a *Adapter) valueToString(value interface{}) (string, error) {
	switch v := value.(type) {
	case string:
		return v, nil
	case json.Number:
		return v.String(), nil
	case bool:
		if v {
			return "true", nil
		}
		return "false", nil
	case nil:
		return "", nil
	default:
		data, err := json.Marshal(value)
		if err != nil {
			return "", err
		}
		return string(data), nil
	}
}

func (a *Adapter) NormalizePath(userPath string, allPaths interface{}) (string, error) {
	var sortedPaths []string

	switch paths := allPaths.(type) {
	case []confpaths.Path:
		sortedPaths = make([]string, len(paths))
		for i, p := range paths {
			sortedPaths[i] = p.String()
		}
	case []showpaths.Path:
		sortedPaths = make([]string, len(paths))
		for i, p := range paths {
			sortedPaths[i] = p.String()
		}
	case []operpaths.Path:
		sortedPaths = make([]string, len(paths))
		for i, p := range paths {
			sortedPaths[i] = p.String()
		}
	default:
		return userPath, nil
	}

	sort.Slice(sortedPaths, func(i, j int) bool {
		iParts := strings.Split(sortedPaths[i], ".")
		jParts := strings.Split(sortedPaths[j], ".")
		return len(iParts) > len(jParts)
	})

	for _, patternStr := range sortedPaths {
		patternParts := strings.Split(patternStr, ".")

		parts, match := a.extractPathParts(userPath, patternParts)
		if !match {
			continue
		}

		values := []string{}
		for i, patternPart := range patternParts {
			if strings.HasPrefix(patternPart, "<") && strings.HasSuffix(patternPart, ">") {
				values = append(values, parts[i])
			}
		}

		encoded, err := paths.Build(patternStr, values...)
		if err != nil {
			continue
		}

		return encoded, nil
	}

	return userPath, nil
}

func (a *Adapter) extractPathParts(userPath string, patternParts []string) ([]string, bool) {
	result := make([]string, len(patternParts))
	remaining := userPath

	for i := 0; i < len(patternParts); i++ {
		if remaining == "" {
			return nil, false
		}

		isWildcard := strings.HasPrefix(patternParts[i], "<") && strings.HasSuffix(patternParts[i], ">")

		if i == len(patternParts)-1 {
			if isWildcard {
				result[i] = remaining
				return result, true
			}
			if remaining == patternParts[i] {
				result[i] = remaining
				return result, true
			}
			return nil, false
		}

		if !isWildcard {
			expected := patternParts[i] + "."
			if !strings.HasPrefix(remaining, expected) {
				return nil, false
			}
			result[i] = patternParts[i]
			remaining = remaining[len(expected):]
			continue
		}

		nextLiteral := patternParts[i+1]
		for j := i + 1; j < len(patternParts); j++ {
			if !strings.HasPrefix(patternParts[j], "<") || !strings.HasSuffix(patternParts[j], ">") {
				nextLiteral = patternParts[j]
				break
			}
		}

		idx := strings.Index(remaining, "."+nextLiteral)
		if idx == -1 {
			return nil, false
		}

		result[i] = remaining[:idx]
		remaining = remaining[idx+1:]
	}

	return result, true
}
