package service

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
	oper.RegisterFactory(NewCreateServiceHandler)
}

type CreateServiceHandler struct {
	deps   *deps.OperDeps
	logger *slog.Logger
}

func NewCreateServiceHandler(deps *deps.OperDeps) oper.OperHandler {
	return &CreateServiceHandler{
		deps:   deps,
		logger: logger.Get(local.Namespace).WithGroup("oper"),
	}
}

func (h *CreateServiceHandler) Execute(ctx context.Context, req *oper.Request) (interface{}, error) {
	provider := local.GetProvider()
	if provider == nil {
		return nil, fmt.Errorf("local auth provider not initialized")
	}

	db := provider.GetDB()

	var createReq local.CreateServiceRequest
	if err := json.Unmarshal(req.Body, &createReq); err != nil {
		return nil, fmt.Errorf("invalid request body: %w", err)
	}

	if createReq.Name == "" {
		return nil, fmt.Errorf("service name is required")
	}

	serviceID, err := local.CreateService(db, createReq.Name, createReq.Description)
	if err != nil {
		return nil, fmt.Errorf("failed to create service: %w", err)
	}

	service, err := local.GetServiceByID(db, serviceID)
	if err != nil {
		return nil, fmt.Errorf("failed to get created service: %w", err)
	}

	h.logger.Info("Service created", "name", createReq.Name, "service_id", service.ID)

	return &local.CreateServiceResponse{
		ServiceID: service.ID,
		Name:      service.Name,
		Message:   "Service created successfully",
	}, nil
}

func (h *CreateServiceHandler) PathPattern() operpaths.Path {
	return operpaths.Path(local.OperCreateServicePath)
}

func (h *CreateServiceHandler) Dependencies() []operpaths.Path {
	return nil
}
