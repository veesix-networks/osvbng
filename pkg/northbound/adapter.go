package northbound

import (
	"context"
	"fmt"

	"github.com/veesix-networks/osvbng/pkg/handlers/conf"
	confpaths "github.com/veesix-networks/osvbng/pkg/handlers/conf/paths"
	conftypes "github.com/veesix-networks/osvbng/pkg/handlers/conf/types"
	"github.com/veesix-networks/osvbng/pkg/handlers/oper"
	operpaths "github.com/veesix-networks/osvbng/pkg/handlers/oper/paths"
	"github.com/veesix-networks/osvbng/pkg/handlers/show"
	showpaths "github.com/veesix-networks/osvbng/pkg/handlers/show/paths"
)

type Adapter struct {
	showRegistry *show.Registry
	confRegistry *conf.Registry
	operRegistry *oper.Registry
}

func NewAdapter(showReg *show.Registry, confReg *conf.Registry, operReg *oper.Registry) *Adapter {
	return &Adapter{
		showRegistry: showReg,
		confRegistry: confReg,
		operRegistry: operReg,
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

	return a.confRegistry.ApplyWithCallbacks(ctx, handler, hctx)
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
