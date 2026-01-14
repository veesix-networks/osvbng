package api

type Status struct {
	State         string `json:"state"`
	ListenAddress string `json:"listen_address"`
	Running       bool   `json:"running"`
}

type PathsResponse struct {
	ShowPaths   []string `json:"show_paths"`
	ConfigPaths []string `json:"config_paths"`
	OperPaths   []string `json:"oper_paths"`
}

type ShowResponse struct {
	Path string      `json:"path"`
	Data interface{} `json:"data"`
}

type ConfigRequest struct {
	Value interface{} `json:"value"`
}

type ErrorResponse struct {
	Error string `json:"error"`
}
