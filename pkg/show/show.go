package show

import "context"

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

type Registry interface {
	GetHandler(path string) (Handler, error)
}
