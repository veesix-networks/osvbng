package system

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/veesix-networks/osvbng/pkg/deps"
	"github.com/veesix-networks/osvbng/pkg/handlers/oper"
	"github.com/veesix-networks/osvbng/pkg/handlers/oper/paths"
	"github.com/veesix-networks/osvbng/pkg/logger"
)

func init() {
	oper.RegisterFactory(NewSetLoggingLevelHandler)
}

type SetLoggingLevelHandler struct {
	deps *deps.OperDeps
}

type SetLoggingLevelRequest struct {
	Level string `json:"level"`
}

type SetLoggingLevelResponse struct {
	Name  string `json:"name"`
	Level string `json:"level"`
}

func NewSetLoggingLevelHandler(deps *deps.OperDeps) oper.OperHandler {
	return &SetLoggingLevelHandler{deps: deps}
}

func (h *SetLoggingLevelHandler) Execute(ctx context.Context, req *oper.Request) (interface{}, error) {
	wildcards, err := h.PathPattern().ExtractWildcards(req.Path, 1)
	if err != nil {
		return nil, err
	}
	name := wildcards[0]

	var body SetLoggingLevelRequest
	if err := json.Unmarshal(req.Body, &body); err != nil {
		return nil, fmt.Errorf("invalid request body: %w", err)
	}

	level := logger.LogLevel(body.Level)
	if level != logger.LogLevelDebug && level != logger.LogLevelInfo &&
		level != logger.LogLevelWarn && level != logger.LogLevelError {
		if body.Level == "" {
			logger.ClearComponentLevel(name)
			return &SetLoggingLevelResponse{Name: name, Level: "default"}, nil
		}
		return nil, fmt.Errorf("invalid level: %s (must be debug, info, warn, error)", body.Level)
	}

	logger.SetComponentLevel(name, level)

	return &SetLoggingLevelResponse{Name: name, Level: body.Level}, nil
}

func (h *SetLoggingLevelHandler) PathPattern() paths.Path {
	return paths.SystemLoggingLevel
}

func (h *SetLoggingLevelHandler) Dependencies() []paths.Path {
	return nil
}
