package nic

func init() {
	Register(Mellanox{})
	Register(Intel{})
	Register(Generic{})
}
