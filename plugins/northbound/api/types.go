package api

import (
	"time"

	"github.com/veesix-networks/osvbng/pkg/handlers/pagination"
)

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

type PaginatedShowResponse struct {
	Path       string          `json:"path"`
	Data       interface{}     `json:"data"`
	Pagination pagination.Meta `json:"pagination"`
}

type ConfigRequest struct {
	Value interface{} `json:"value"`
}

type OperationResponse struct {
	Status  string `json:"status"`
	Message string `json:"message,omitempty"`
	Version int    `json:"version,omitempty"`
}

type ConfigSessionCreateResponse struct {
	SessionID string `json:"session_id"`
}

type ConfigHistoryResponse struct {
	Versions []ConfigVersionResponse `json:"versions"`
}

type ConfigVersionResponse struct {
	Version   int                    `json:"version"`
	Timestamp time.Time              `json:"timestamp"`
	CommitMsg string                 `json:"commit_msg,omitempty"`
	Changes   []ConfigChangeResponse `json:"changes,omitempty"`
}

type ConfigChangeResponse struct {
	Type  string `json:"type"`
	Path  string `json:"path"`
	Value string `json:"value,omitempty"`
}

type DiffResponse struct {
	Added    []DiffLineResponse `json:"added,omitempty"`
	Deleted  []DiffLineResponse `json:"deleted,omitempty"`
	Modified []DiffLineResponse `json:"modified,omitempty"`
}

type DiffLineResponse struct {
	Path  string `json:"path"`
	Value string `json:"value,omitempty"`
}

type ErrorResponse struct {
	Error string `json:"error"`
}
