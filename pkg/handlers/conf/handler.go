package conf

import (
	"context"
	"fmt"
	"strings"

	"github.com/veesix-networks/osvbng/pkg/deps"
	"github.com/veesix-networks/osvbng/pkg/handlers/conf/paths"
	"github.com/veesix-networks/osvbng/pkg/handlers/conf/types"
)

type HandlerContext struct {
	SessionID types.SessionID
	Path      string
	OldValue  interface{}
	NewValue  interface{}

	frrReloadNeeded bool
}

func (hctx *HandlerContext) MarkFRRReloadNeeded() {
	hctx.frrReloadNeeded = true
}

func (hctx *HandlerContext) IsFRRReloadNeeded() bool {
	return hctx.frrReloadNeeded
}

type Callbacks struct {
	OnBeforeValidate func(hctx *HandlerContext) error
	OnAfterValidate  func(hctx *HandlerContext, err error)
	OnBeforeApply    func(hctx *HandlerContext) error
	OnAfterApply     func(hctx *HandlerContext, err error)
	OnBeforeRollback func(hctx *HandlerContext) error
	OnAfterRollback  func(hctx *HandlerContext, err error)
}

type Handler interface {
	Validate(ctx context.Context, hctx *HandlerContext) error
	Apply(ctx context.Context, hctx *HandlerContext) error
	Rollback(ctx context.Context, hctx *HandlerContext) error
	PathPattern() paths.Path
	Dependencies() []paths.Path
	Callbacks() *Callbacks
}

type HandlerFactory func(deps *deps.ConfDeps) Handler

var factories []HandlerFactory

func RegisterFactory(factory HandlerFactory) {
	factories = append(factories, factory)
}

type Registry struct {
	handlers  map[string]Handler
	callbacks *Callbacks
}

func NewRegistry() *Registry {
	return &Registry{
		handlers:  make(map[string]Handler),
		callbacks: &Callbacks{},
	}
}

func (r *Registry) SetCallbacks(cb *Callbacks) {
	r.callbacks = cb
}

func (r *Registry) AutoRegisterAll(d *deps.ConfDeps) {
	r.handlers = make(map[string]Handler)

	for _, factory := range factories {
		handler := factory(d)
		r.MustRegister(handler)
	}
}

func (r *Registry) Register(handler Handler) error {
	path := handler.PathPattern().String()

	if _, exists := r.handlers[path]; exists {
		return fmt.Errorf("conf handler conflict: path '%s' already registered", path)
	}

	r.handlers[path] = handler
	return nil
}

func (r *Registry) MustRegister(handler Handler) {
	if err := r.Register(handler); err != nil {
		panic(err)
	}
}

func (r *Registry) GetAllPaths() []paths.Path {
	paths := make([]paths.Path, 0, len(r.handlers))
	for _, handler := range r.handlers {
		paths = append(paths, handler.PathPattern())
	}
	return paths
}

func (r *Registry) GetHandler(path string) (Handler, error) {
	if handler, ok := r.handlers[path]; ok {
		return handler, nil
	}

	for pattern, handler := range r.handlers {
		if matchPattern(pattern, path) {
			return handler, nil
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

func (r *Registry) ValidateWithCallbacks(ctx context.Context, handler Handler, hctx *HandlerContext) error {
	if r.callbacks.OnBeforeValidate != nil {
		if err := r.callbacks.OnBeforeValidate(hctx); err != nil {
			return err
		}
	}

	if handlerCB := handler.Callbacks(); handlerCB != nil && handlerCB.OnBeforeValidate != nil {
		if err := handlerCB.OnBeforeValidate(hctx); err != nil {
			return err
		}
	}

	err := handler.Validate(ctx, hctx)

	if handlerCB := handler.Callbacks(); handlerCB != nil && handlerCB.OnAfterValidate != nil {
		handlerCB.OnAfterValidate(hctx, err)
	}

	if r.callbacks.OnAfterValidate != nil {
		r.callbacks.OnAfterValidate(hctx, err)
	}

	return err
}

func (r *Registry) ApplyWithCallbacks(ctx context.Context, handler Handler, hctx *HandlerContext) error {
	if r.callbacks.OnBeforeApply != nil {
		if err := r.callbacks.OnBeforeApply(hctx); err != nil {
			return err
		}
	}

	if handlerCB := handler.Callbacks(); handlerCB != nil && handlerCB.OnBeforeApply != nil {
		if err := handlerCB.OnBeforeApply(hctx); err != nil {
			return err
		}
	}

	err := handler.Apply(ctx, hctx)

	if handlerCB := handler.Callbacks(); handlerCB != nil && handlerCB.OnAfterApply != nil {
		handlerCB.OnAfterApply(hctx, err)
	}

	if r.callbacks.OnAfterApply != nil {
		r.callbacks.OnAfterApply(hctx, err)
	}

	return err
}

func (r *Registry) RollbackWithCallbacks(ctx context.Context, handler Handler, hctx *HandlerContext) error {
	if r.callbacks.OnBeforeRollback != nil {
		if err := r.callbacks.OnBeforeRollback(hctx); err != nil {
			return err
		}
	}

	if handlerCB := handler.Callbacks(); handlerCB != nil && handlerCB.OnBeforeRollback != nil {
		if err := handlerCB.OnBeforeRollback(hctx); err != nil {
			return err
		}
	}

	err := handler.Rollback(ctx, hctx)

	if handlerCB := handler.Callbacks(); handlerCB != nil && handlerCB.OnAfterRollback != nil {
		handlerCB.OnAfterRollback(hctx, err)
	}

	if r.callbacks.OnAfterRollback != nil {
		r.callbacks.OnAfterRollback(hctx, err)
	}

	return err
}

func ExtractInterfaceName(path string) string {
	parts := strings.Split(path, ".")
	if len(parts) >= 2 && parts[0] == "interfaces" {
		return parts[1]
	}
	return ""
}
