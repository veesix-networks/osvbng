package northbound

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/veesix-networks/osvbng/pkg/configmgr"
	"github.com/veesix-networks/osvbng/pkg/handlers/conf"
	confpaths "github.com/veesix-networks/osvbng/pkg/handlers/conf/paths"
	conftypes "github.com/veesix-networks/osvbng/pkg/handlers/conf/types"
	"github.com/veesix-networks/osvbng/pkg/handlers/oper"
	operpaths "github.com/veesix-networks/osvbng/pkg/handlers/oper/paths"
	"github.com/veesix-networks/osvbng/pkg/handlers/show"
	showpaths "github.com/veesix-networks/osvbng/pkg/handlers/show/paths"
	"github.com/veesix-networks/osvbng/pkg/paths"
)

type Adapter struct {
	showRegistry *show.Registry
	confRegistry *conf.Registry
	operRegistry *oper.Registry
	configMgr    *configmgr.ConfigManager
}

func NewAdapter(showReg *show.Registry, confReg *conf.Registry, operReg *oper.Registry, configMgr *configmgr.ConfigManager) *Adapter {
	return &Adapter{
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
	handler, err := a.operRegistry.GetHandler(path)
	if err != nil {
		return nil, fmt.Errorf("oper handler not found for path %s: %w", path, err)
	}

	req := &oper.Request{
		Path:    path,
		Body:    body,
		Options: options,
	}

	return handler.Execute(ctx, req)
}

func (a *Adapter) ExecuteShow(ctx context.Context, path string, options map[string]string) (interface{}, error) {
	handler, err := a.showRegistry.GetHandler(path)
	if err != nil {
		return nil, fmt.Errorf("show handler not found for path %s: %w", path, err)
	}

	req := &show.Request{
		Path:    path,
		Options: options,
	}

	return handler.Collect(ctx, req)
}

func (a *Adapter) ValidateConfig(ctx context.Context, sessionID conftypes.SessionID, path string, value interface{}) error {
	handler, err := a.confRegistry.GetHandler(path)
	if err != nil {
		return fmt.Errorf("config handler not found for path %s: %w", path, err)
	}

	hctx := &conf.HandlerContext{
		SessionID: sessionID,
		Path:      path,
		NewValue:  value,
	}

	return a.confRegistry.ValidateWithCallbacks(ctx, handler, hctx)
}

func (a *Adapter) ApplyConfig(ctx context.Context, sessionID conftypes.SessionID, path string, oldValue, newValue interface{}) error {
	handler, err := a.confRegistry.GetHandler(path)
	if err != nil {
		return fmt.Errorf("config handler not found for path %s: %w", path, err)
	}

	hctx := &conf.HandlerContext{
		SessionID: sessionID,
		Path:      path,
		OldValue:  oldValue,
		NewValue:  newValue,
	}

	if err := a.confRegistry.ApplyWithCallbacks(ctx, handler, hctx); err != nil {
		return err
	}

	if hctx.IsFRRReloadNeeded() {
		if err := a.configMgr.ReloadFRR(); err != nil {
			return fmt.Errorf("FRR reload failed: %w", err)
		}
	}

	return nil
}

func (a *Adapter) RollbackConfig(ctx context.Context, sessionID conftypes.SessionID, path string, oldValue, newValue interface{}) error {
	handler, err := a.confRegistry.GetHandler(path)
	if err != nil {
		return fmt.Errorf("config handler not found for path %s: %w", path, err)
	}

	hctx := &conf.HandlerContext{
		SessionID: sessionID,
		Path:      path,
		OldValue:  oldValue,
		NewValue:  newValue,
	}

	return a.confRegistry.RollbackWithCallbacks(ctx, handler, hctx)
}

func (a *Adapter) HasOperHandler(path string) bool {
	_, err := a.operRegistry.GetHandler(path)
	return err == nil
}

func (a *Adapter) GetRunningConfig(ctx context.Context) (*conftypes.Config, error) {
	return a.configMgr.GetRunning()
}

func (a *Adapter) GetStartupConfig(ctx context.Context) (*conftypes.Config, error) {
	return a.configMgr.GetStartup()
}

func (a *Adapter) SetAndCommit(ctx context.Context, path string, value interface{}) error {
	sessionID, err := a.configMgr.CreateCandidateSession()
	if err != nil {
		return fmt.Errorf("failed to create candidate session: %w", err)
	}

	pathValues, err := a.flattenValue(path, value)
	if err != nil {
		return fmt.Errorf("failed to flatten value: %w", err)
	}

	for _, pv := range pathValues {
		if err := a.configMgr.Set(sessionID, pv.path, pv.value); err != nil {
			return fmt.Errorf("failed to set %s: %w", pv.path, err)
		}
	}

	if err := a.configMgr.Commit(sessionID); err != nil {
		return fmt.Errorf("failed to commit: %w", err)
	}

	return nil
}

type pathValue struct {
	path  string
	value string
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

	valueStr, err := a.valueToString(value)
	if err != nil {
		return nil, err
	}
	result = append(result, pathValue{path: basePath, value: valueStr})
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

func (a *Adapter) NormalizePath(userPath string) (string, error) {
	allPaths := a.confRegistry.GetAllPaths()

	sortedPaths := make([]string, len(allPaths))
	for i, p := range allPaths {
		sortedPaths[i] = p.String()
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

		fmt.Printf("DEBUG NormalizePath: matched pattern=%s, parts=%v\n", patternStr, parts)

		values := []string{}
		for i, patternPart := range patternParts {
			if strings.HasPrefix(patternPart, "<") && strings.HasSuffix(patternPart, ">") {
				values = append(values, parts[i])
			}
		}

		fmt.Printf("DEBUG NormalizePath: values=%v\n", values)

		encoded, err := paths.Build(patternStr, values...)
		if err != nil {
			fmt.Printf("DEBUG NormalizePath: build error=%v\n", err)
			continue
		}

		fmt.Printf("DEBUG NormalizePath: encoded=%s\n", encoded)
		return encoded, nil
	}

	fmt.Printf("DEBUG NormalizePath: no match for path=%s\n", userPath)
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
