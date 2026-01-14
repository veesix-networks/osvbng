package provider

type Info struct {
	Name    string
	Version string
	Author  string
}

type Provider interface {
	Info() Info
}
