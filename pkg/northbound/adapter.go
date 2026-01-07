package northbound

import (
	"context"
	"fmt"

	confhandlers "github.com/veesix-networks/osvbng/pkg/conf/handlers"
	"github.com/veesix-networks/osvbng/pkg/conf/paths"
	conftypes "github.com/veesix-networks/osvbng/pkg/conf/types"
	"github.com/veesix-networks/osvbng/pkg/oper"
	operhandlers "github.com/veesix-networks/osvbng/pkg/oper/handlers"
	operpaths "github.com/veesix-networks/osvbng/pkg/oper/paths"
	"github.com/veesix-networks/osvbng/pkg/show"
	showhandlers "github.com/veesix-networks/osvbng/pkg/show/handlers"
	showpaths "github.com/veesix-networks/osvbng/pkg/show/paths"
)

type Adapter struct {
	showRegistry *showhandlers.Registry
	confRegistry *confhandlers.Registry
	operRegistry *operhandlers.Registry
}

func NewAdapter(showReg *showhandlers.Registry, confReg *confhandlers.Registry, operReg *operhandlers.Registry) *Adapter {
	return &Adapter{
		showRegistry: showReg,
		confRegistry: confReg,
		operRegistry: operReg,
	}
}

func (a *Adapter) GetAllShowPaths() []showpaths.Path {
	return a.showRegistry.GetAllPaths()
}

func (a *Adapter) GetAllConfPaths() []paths.Path {
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

	hctx := &confhandlers.HandlerContext{
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

	hctx := &confhandlers.HandlerContext{
		SessionID: sessionID,
		Path:      path,
		OldValue:  oldValue,
		NewValue:  newValue,
	}

	return a.confRegistry.ApplyWithCallbacks(ctx, handler, hctx)
}

func (a *Adapter) RollbackConfig(ctx context.Context, sessionID conftypes.SessionID, path string, oldValue, newValue interface{}) error {
	handler, err := a.confRegistry.GetHandler(path)
	if err != nil {
		return fmt.Errorf("config handler not found for path %s: %w", path, err)
	}

	hctx := &confhandlers.HandlerContext{
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
