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
	c.writeJSON(w, resp)
}

func (c *Component) handleShow(w http.ResponseWriter, r *http.Request) {
	path := r.PathValue("path")
	if path == "" {
		c.writeError(w, http.StatusBadRequest, "path required")
		return
	}

	path = strings.ReplaceAll(path, "/", ".")

	showPath, err := c.adapter.NormalizePath(path, c.adapter.GetAllShowPaths())
	if err != nil {
		c.writeError(w, http.StatusBadRequest, "failed to normalize path: "+err.Error())
		return
	}

	options := make(map[string]string)
	for key, values := range r.URL.Query() {
		if len(values) > 0 {
			options[key] = values[0]
		}
	}

	data, err := c.adapter.ExecuteShow(r.Context(), showPath, options)
	if err != nil {
		c.logger.Error("show handler failed", "path", showPath, "error", err)
		c.writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	resp := ShowResponse{
		Path: showPath,
		Data: data,
	}

	w.Header().Set("Content-Type", "application/json")
	c.writeJSON(w, resp)
}

func (c *Component) handleSet(w http.ResponseWriter, r *http.Request) {
	path := r.PathValue("path")
	if path == "" {
		c.writeError(w, http.StatusBadRequest, "path required")
		return
	}

	path = strings.ReplaceAll(path, "/", ".")

	normalizedPath, err := c.adapter.NormalizePath(path, c.adapter.GetAllConfPaths())
	if err != nil {
		c.writeError(w, http.StatusBadRequest, "failed to normalize path: "+err.Error())
		return
	}

	var value interface{}
	decoder := json.NewDecoder(r.Body)
	if err := decoder.Decode(&value); err != nil {
		c.writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	if err := c.adapter.SetAndCommit(r.Context(), normalizedPath, value); err != nil {
		c.logger.Error("config set and commit failed", "path", normalizedPath, "error", err)
		c.writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	c.writeJSON(w, map[string]string{"status": "ok"})
}

func (c *Component) handleExec(w http.ResponseWriter, r *http.Request) {
	path := r.PathValue("path")
	if path == "" {
		c.writeError(w, http.StatusBadRequest, "path required")
		return
	}

	path = strings.ReplaceAll(path, "/", ".")

	operPath, err := c.adapter.NormalizePath(path, c.adapter.GetAllOperPaths())
	if err != nil {
		c.writeError(w, http.StatusBadRequest, "failed to normalize path: "+err.Error())
		return
	}

	var body []byte
	if r.Body != nil {
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
	c.writeJSON(w, data)
}

func (c *Component) writeError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	c.writeJSON(w, ErrorResponse{Error: message})
}

func (c *Component) writeJSON(w http.ResponseWriter, v interface{}) {
	encoder := json.NewEncoder(w)
	encoder.SetEscapeHTML(false)
	encoder.Encode(v)
}

func (c *Component) handleRunningConfig(w http.ResponseWriter, r *http.Request) {
	cfg, err := c.adapter.GetRunningConfig(r.Context())
	if err != nil {
		c.writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	w.Header().Set("Content-Type", "application/json")
	c.writeJSON(w, cfg)
}

func (c *Component) handleStartupConfig(w http.ResponseWriter, r *http.Request) {
	cfg, err := c.adapter.GetStartupConfig(r.Context())
	if err != nil {
		c.writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	w.Header().Set("Content-Type", "application/json")
	c.writeJSON(w, cfg)
}
