package show

import (
	"context"
	"fmt"

	"github.com/veesix-networks/osvbng/pkg/deps"
	"github.com/veesix-networks/osvbng/pkg/handlers/show/paths"
)

type Handler interface {
	Collect(ctx context.Context, req *Request) (interface{}, error)
}

type Request struct {
	Path    string
	Format  int
	Options map[string]string
}

func (r *Request) GetPath() string {
	return r.Path
}

type ShowHandler interface {
	Collect(ctx context.Context, req *Request) (interface{}, error)
	PathPattern() paths.Path
	Dependencies() []paths.Path
}

type HandlerFactory func(deps *deps.ShowDeps) ShowHandler

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

func (r *Registry) AutoRegisterAll(d *deps.ShowDeps) {
	for _, factory := range factories {
		handler := factory(d)
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

func (r *Registry) GetHandler(path string) (Handler, error) {
	if handler, ok := r.handlers[path]; ok {
		return &handlerAdapter{h: handler}, nil
	}
	return nil, fmt.Errorf("no handler registered for path: %s", path)
}

func (r *Registry) GetAllPaths() []paths.Path {
	allPaths := make([]paths.Path, 0, len(r.handlers))
	for _, handler := range r.handlers {
		allPaths = append(allPaths, handler.PathPattern())
	}
	return allPaths
}

type handlerAdapter struct {
	h ShowHandler
}

func (a *handlerAdapter) Collect(ctx context.Context, req *Request) (interface{}, error) {
	return a.h.Collect(ctx, req)
}
