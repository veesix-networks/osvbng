package oper

import "context"

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

type Registry interface {
	GetHandler(path string) (Handler, error)
}
