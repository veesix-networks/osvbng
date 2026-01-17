package oper

import (
	"context"
	"fmt"
	"strings"

	"github.com/veesix-networks/osvbng/pkg/deps"
	"github.com/veesix-networks/osvbng/pkg/handlers/oper/paths"
)

type Handler interface {
	Execute(ctx context.Context, req *Request) (interface{}, error)
}

type Request struct {
	Path    string
	Body    []byte
	Options map[string]string
}

func (r *Request) GetPath() string {
	return r.Path
}

type OperHandler interface {
	Execute(ctx context.Context, req *Request) (interface{}, error)
	PathPattern() paths.Path
	Dependencies() []paths.Path
}

type HandlerFactory func(deps *deps.OperDeps) OperHandler

var factories []HandlerFactory

func RegisterFactory(factory HandlerFactory) {
	factories = append(factories, factory)
}

type Registry struct {
	handlers map[string]OperHandler
}

func NewRegistry() *Registry {
	return &Registry{
		handlers: make(map[string]OperHandler),
	}
}

func (r *Registry) AutoRegisterAll(d *deps.OperDeps) {
	for _, factory := range factories {
		handler := factory(d)
		path := handler.PathPattern().String()

		if _, exists := r.handlers[path]; exists {
			continue
		}

		r.MustRegister(handler)
	}
}

func (r *Registry) Register(handler OperHandler) error {
	path := handler.PathPattern().String()

	if _, exists := r.handlers[path]; exists {
		return fmt.Errorf("oper handler conflict: path '%s' already registered", path)
	}

	r.handlers[path] = handler
	return nil
}

func (r *Registry) MustRegister(handler OperHandler) {
	if err := r.Register(handler); err != nil {
		panic(err)
	}
}

func (r *Registry) GetHandler(path string) (Handler, error) {
	if handler, ok := r.handlers[path]; ok {
		return &handlerAdapter{h: handler}, nil
	}

	for pattern, handler := range r.handlers {
		if matchPattern(pattern, path) {
			return &handlerAdapter{h: handler}, nil
		}
	}

	return nil, fmt.Errorf("no handler registered for path: %s", path)
}

func matchPattern(pattern, path string) bool {
	patternParts := strings.Split(pattern, ".")
	pathParts := strings.Split(path, ".")

	if len(patternParts) != len(pathParts) {
		return false
	}

	for i := range patternParts {
		if isWildcard(patternParts[i]) {
			continue
		}
		if patternParts[i] != pathParts[i] {
			return false
		}
	}

	return true
}

func isWildcard(part string) bool {
	return strings.HasPrefix(part, "<") && strings.HasSuffix(part, ">")
}

func (r *Registry) GetAllPaths() []paths.Path {
	allPaths := make([]paths.Path, 0, len(r.handlers))
	for _, handler := range r.handlers {
		allPaths = append(allPaths, handler.PathPattern())
	}
	return allPaths
}

type handlerAdapter struct {
	h OperHandler
}

func (a *handlerAdapter) Execute(ctx context.Context, req *Request) (interface{}, error) {
	return a.h.Execute(ctx, req)
}
