package attribute

import (
	"github.com/veesix-networks/osvbng/pkg/deps"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/veesix-networks/osvbng/pkg/handlers/oper"
	operpaths "github.com/veesix-networks/osvbng/pkg/handlers/oper/paths"
	"github.com/veesix-networks/osvbng/pkg/logger"
	"github.com/veesix-networks/osvbng/plugins/auth/local"
)

func init() {
	oper.RegisterFactory(NewSetServiceAttributeHandler)
}

type SetServiceAttributeHandler struct {
	deps   *deps.OperDeps
	logger *slog.Logger
}



func NewSetServiceAttributeHandler(deps *deps.OperDeps) oper.OperHandler {
	return &SetServiceAttributeHandler{
		deps:   deps,
		logger: logger.Component(local.Namespace + ".oper"),
	}
}

func (h *SetServiceAttributeHandler) Execute(ctx context.Context, req *oper.Request) (interface{}, error) {
	provider := local.GetProvider()
	if provider == nil {
		return nil, fmt.Errorf("local auth provider not initialized")
	}

	db := provider.GetDB()

	wildcards, err := h.PathPattern().ExtractWildcards(req.Path, 1)
	if err != nil {
		return nil, err
	}
	serviceName := wildcards[0]

	var setReq local.SetServiceAttributeRequest
	if err := json.Unmarshal(req.Body, &setReq); err != nil {
		return nil, fmt.Errorf("invalid request body: %w", err)
	}

	if setReq.Attribute == "" || setReq.Value == "" || setReq.Op == "" {
		return nil, fmt.Errorf("attribute, value and op are required")
	}

	service, err := local.GetServiceByName(db, serviceName)
	if err != nil {
		return nil, fmt.Errorf("service not found: %w", err)
	}

	if err := local.SetAttributeByID(db, local.EntityTypeService, service.ID, local.AttributeTypeResponse, setReq.Attribute, setReq.Value, setReq.Op); err != nil {
		return nil, fmt.Errorf("failed to set service attribute: %w", err)
	}

	h.logger.Info("Service attribute set", "service", serviceName, "attribute", setReq.Attribute, "value", setReq.Value, "op", setReq.Op)

	return &local.OperResponse{
		Message: fmt.Sprintf("Attribute '%s' set for service '%s'", setReq.Attribute, serviceName),
	}, nil
}

func (h *SetServiceAttributeHandler) PathPattern() operpaths.Path {
	return operpaths.Path(local.OperSetServiceAttributePath)
}

func (h *SetServiceAttributeHandler) Dependencies() []operpaths.Path {
	return nil
}
