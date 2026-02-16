package southbound

type Routing interface {
	AddLocalRoute(prefix string) error
}
