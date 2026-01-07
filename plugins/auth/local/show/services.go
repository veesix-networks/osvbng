package show

import (
	"context"
	"fmt"

	"github.com/veesix-networks/osvbng/pkg/show/handlers"
	"github.com/veesix-networks/osvbng/pkg/show/paths"
	"github.com/veesix-networks/osvbng/plugins/auth/local"
)

func init() {
	handlers.RegisterFactory(NewServicesHandler)
	handlers.RegisterFactory(NewServiceHandler)
}

type ServicesHandler struct {
	deps *handlers.ShowDeps
}

type ServiceHandler struct {
	deps *handlers.ShowDeps
}

type ServicesResponse struct {
	Services []ServiceInfo `json:"services"`
}

type ServiceInfo struct {
	Name        string            `json:"name"`
	Description string            `json:"description"`
	Attributes  map[string]string `json:"attributes,omitempty"`
	CreatedAt   string            `json:"created_at"`
}

func NewServicesHandler(deps *handlers.ShowDeps) handlers.ShowHandler {
	return &ServicesHandler{deps: deps}
}

func NewServiceHandler(deps *handlers.ShowDeps) handlers.ShowHandler {
	return &ServiceHandler{deps: deps}
}

func (h *ServicesHandler) Collect(ctx context.Context, req *handlers.ShowRequest) (interface{}, error) {
	provider := local.GetProvider()
	if provider == nil {
		return nil, fmt.Errorf("local auth provider not initialized")
	}

	db := provider.GetDB()

	services, err := local.ListServices(db)
	if err != nil {
		return nil, err
	}

	var serviceInfos []ServiceInfo
	for _, service := range services {
		serviceInfos = append(serviceInfos, ServiceInfo{
			Name:        service.Name,
			Description: service.Description,
			CreatedAt:   service.CreatedAt.Format("2006-01-02 15:04:05"),
		})
	}

	return &ServicesResponse{Services: serviceInfos}, nil
}

func (h *ServicesHandler) PathPattern() paths.Path {
	return paths.Path(local.ShowServicesPath)
}

func (h *ServicesHandler) Dependencies() []paths.Path {
	return nil
}

func (h *ServiceHandler) Collect(ctx context.Context, req *handlers.ShowRequest) (interface{}, error) {
	provider := local.GetProvider()
	if provider == nil {
		return nil, fmt.Errorf("local auth provider not initialized")
	}

	db := provider.GetDB()

	serviceName := req.Options["name"]
	if serviceName == "" {
		return nil, fmt.Errorf("service name required")
	}

	service, err := local.GetServiceByName(db, serviceName)
	if err != nil {
		return nil, err
	}

	attributes, err := local.GetAttributes(db, local.EntityTypeService, serviceName, local.AttributeTypeResponse)
	if err != nil {
		return nil, err
	}

	attrs := make(map[string]string)
	for _, attr := range attributes {
		attrs[attr.AttributeName] = attr.AttributeValue
	}

	return &ServiceInfo{
		Name:        service.Name,
		Description: service.Description,
		Attributes:  attrs,
		CreatedAt:   service.CreatedAt.Format("2006-01-02 15:04:05"),
	}, nil
}

func (h *ServiceHandler) PathPattern() paths.Path {
	return paths.Path(local.ShowServicePath)
}

func (h *ServiceHandler) Dependencies() []paths.Path {
	return nil
}
