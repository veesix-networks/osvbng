package api

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
)

func (c *Component) handlePaths(w http.ResponseWriter, r *http.Request) {
	showPaths := c.adapter.GetAllShowPaths()
	confPaths := c.adapter.GetAllConfPaths()
	operPaths := c.adapter.GetAllOperPaths()

	showPathStrs := make([]string, len(showPaths))
	for i, p := range showPaths {
		showPathStrs[i] = p.String()
	}

	confPathStrs := make([]string, len(confPaths))
	for i, p := range confPaths {
		confPathStrs[i] = p.String()
	}

	operPathStrs := make([]string, len(operPaths))
	for i, p := range operPaths {
		operPathStrs[i] = p.String()
	}

	resp := PathsResponse{
		ShowPaths:   showPathStrs,
		ConfigPaths: confPathStrs,
		OperPaths:   operPathStrs,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func (c *Component) handleShow(w http.ResponseWriter, r *http.Request) {
	urlPath := r.PathValue("path")
	if urlPath == "" {
		c.writeError(w, http.StatusBadRequest, "path required")
		return
	}

	internalPath := strings.ReplaceAll(urlPath, "/", ".")

	options := make(map[string]string)
	for key, values := range r.URL.Query() {
		if len(values) > 0 {
			options[key] = values[0]
		}
	}

	data, err := c.adapter.ExecuteShow(r.Context(), internalPath, options)
	if err != nil {
		c.logger.Error("show handler failed", "path", internalPath, "error", err)
		c.writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	resp := ShowResponse{
		Path: internalPath,
		Data: data,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func (c *Component) handleConfig(w http.ResponseWriter, r *http.Request) {
	path := r.PathValue("path")
	if path == "" {
		c.writeError(w, http.StatusBadRequest, "path required")
		return
	}

	path = strings.ReplaceAll(path, "/", ".")

	// Check if there's an oper handler for this path
	if c.adapter.HasOperHandler(path) {
		c.handleOper(w, r, path)
		return
	}

	var value interface{}
	if err := json.NewDecoder(r.Body).Decode(&value); err != nil {
		c.writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	if err := c.adapter.ValidateConfig(r.Context(), "", path, value); err != nil {
		c.logger.Error("config validation failed", "path", path, "error", err)
		c.writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	if err := c.adapter.ApplyConfig(r.Context(), "", path, nil, value); err != nil {
		c.logger.Error("config apply failed", "path", path, "error", err)
		c.writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func (c *Component) handleOper(w http.ResponseWriter, r *http.Request, operPath string) {
	var body []byte
	if r.Body != nil {
		var err error
		body, err = io.ReadAll(r.Body)
		if err != nil {
			c.writeError(w, http.StatusBadRequest, "failed to read request body")
			return
		}
	}

	options := make(map[string]string)
	for key, values := range r.URL.Query() {
		if len(values) > 0 {
			options[key] = values[0]
		}
	}

	data, err := c.adapter.ExecuteOper(r.Context(), operPath, body, options)
	if err != nil {
		c.logger.Error("oper handler failed", "path", operPath, "error", err)
		c.writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(data)
}

func (c *Component) writeError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(ErrorResponse{Error: message})
}
