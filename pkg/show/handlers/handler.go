package handlers

import (
	"context"
	"fmt"

	"github.com/veesix-networks/osvbng/internal/routing"
	"github.com/veesix-networks/osvbng/internal/subscriber"
	"github.com/veesix-networks/osvbng/pkg/cache"
	"github.com/veesix-networks/osvbng/pkg/component"
	"github.com/veesix-networks/osvbng/pkg/show/paths"
	"github.com/veesix-networks/osvbng/pkg/southbound"
)

type OutputFormat int

const (
	FormatCLI OutputFormat = iota
	FormatJSON
	FormatXML
)

type ShowRequest struct {
	Path    string
	Format  OutputFormat
	Options map[string]string
}

type ShowHandler interface {
	Collect(ctx context.Context, req *ShowRequest) (interface{}, error)
	PathPattern() paths.Path
	Dependencies() []paths.Path
}

type ShowDeps struct {
	Subscriber       *subscriber.Component
	Southbound       *southbound.VPP
	Routing          *routing.Component
	Cache            cache.Cache
	PluginComponents map[string]component.Component
}

type HandlerFactory func(deps *ShowDeps) ShowHandler

var factories []HandlerFactory

func RegisterFactory(factory HandlerFactory) {
	factories = append(factories, factory)
}

type Registry struct {
	handlers map[string]ShowHandler
}

func NewRegistry() *Registry {
	return &Registry{
		handlers: make(map[string]ShowHandler),
	}
}

func (r *Registry) AutoRegisterAll(deps *ShowDeps) {
	for _, factory := range factories {
		handler := factory(deps)
		path := handler.PathPattern().String()

		if _, exists := r.handlers[path]; exists {
			continue
		}

		r.MustRegister(handler)
	}
}

func (r *Registry) Register(handler ShowHandler) error {
	path := handler.PathPattern().String()

	if _, exists := r.handlers[path]; exists {
		return fmt.Errorf("show handler conflict: path '%s' already registered", path)
	}

	r.handlers[path] = handler
	return nil
}

func (r *Registry) MustRegister(handler ShowHandler) {
	if err := r.Register(handler); err != nil {
		panic(err)
	}
}

func (r *Registry) GetHandler(path string) (ShowHandler, error) {
	if handler, ok := r.handlers[path]; ok {
		return handler, nil
	}
	return nil, fmt.Errorf("no handler registered for path: %s", path)
}
