package api

import (
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/veesix-networks/osvbng/pkg/configmgr"
	confhandler "github.com/veesix-networks/osvbng/pkg/handlers/conf"
	"github.com/veesix-networks/osvbng/pkg/handlers/pagination"
	"github.com/veesix-networks/osvbng/pkg/handlers/show"
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

	pageReq := pagination.RequestFromQuery(r.URL.Query())
	sortKey := ""
	if h := c.adapter.LookupShowHandler(showPath); h != nil {
		if sh, ok := h.(show.ShowSortHandler); ok {
			sortKey = sh.SortKey()
		}
	}

	page, err := pagination.Paginate(data, pageReq, sortKey)
	if err != nil {
		c.logger.Warn("pagination sort failed; falling back to unsorted", "path", showPath, "sort_key", sortKey, "error", err)
		page, _ = pagination.Paginate(data, pageReq, "")
	}

	w.Header().Set("Content-Type", "application/json")
	if page.Paginated {
		c.writeJSON(w, PaginatedShowResponse{
			Path:       showPath,
			Data:       page.Items,
			Pagination: page.Meta,
		})
		return
	}

	c.writeJSON(w, ShowResponse{
		Path: showPath,
		Data: data,
	})
}

func (c *Component) handleSet(w http.ResponseWriter, r *http.Request) {
	path := r.PathValue("path")
	if path == "" {
		c.writeError(w, http.StatusBadRequest, "path required")
		return
	}

	path = strings.ReplaceAll(path, "/", ".")
	value, err := decodeJSONBody(r)
	if err != nil {
		c.writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	if err := c.adapter.SetAndCommit(r.Context(), path, value); err != nil {
		c.logger.Error("config set and commit failed", "path", path, "error", err)
		c.writeError(w, statusCodeForConfigError(err), err.Error())
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	c.writeJSON(w, OperationResponse{Status: "ok"})
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

func (c *Component) handleShowRunningConfig(w http.ResponseWriter, r *http.Request) {
	cfg, err := c.adapter.GetRunningConfig(r.Context())
	if err != nil {
		c.writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	c.writeShowResponse(w, "running-config", cfg)
}

func (c *Component) handleShowStartupConfig(w http.ResponseWriter, r *http.Request) {
	cfg, err := c.adapter.GetStartupConfig(r.Context())
	if err != nil {
		c.writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	c.writeShowResponse(w, "startup-config", cfg)
}

func (c *Component) handleShowConfigHistory(w http.ResponseWriter, r *http.Request) {
	versions, err := c.adapter.GetConfigHistory(r.Context())
	if err != nil {
		c.writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	c.writeShowResponse(w, "config.history", ConfigHistoryResponse{
		Versions: convertConfigVersions(versions),
	})
}

func (c *Component) handleShowConfigVersion(w http.ResponseWriter, r *http.Request) {
	versionStr := r.PathValue("version")
	version, err := strconv.Atoi(versionStr)
	if err != nil {
		c.writeError(w, http.StatusBadRequest, "version must be an integer")
		return
	}

	cfgVersion, err := c.adapter.GetConfigVersion(r.Context(), version)
	if err != nil {
		c.writeError(w, statusCodeForConfigError(err), err.Error())
		return
	}

	c.writeShowResponse(w, "config.version", convertConfigVersion(cfgVersion))
}

func (c *Component) handleConfigSessionCreate(w http.ResponseWriter, r *http.Request) {
	sessionID, err := c.adapter.CreateCandidateSession(r.Context())
	if err != nil {
		c.writeError(w, statusCodeForConfigError(err), err.Error())
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	c.writeJSON(w, ConfigSessionCreateResponse{SessionID: string(sessionID)})
}

func (c *Component) handleConfigSessionSet(w http.ResponseWriter, r *http.Request) {
	sessionID := confhandler.SessionID(r.PathValue("session_id"))
	path := r.PathValue("path")
	if path == "" {
		c.writeError(w, http.StatusBadRequest, "path required")
		return
	}

	value, err := decodeJSONBody(r)
	if err != nil {
		c.writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	path = strings.ReplaceAll(path, "/", ".")
	if err := c.adapter.SetCandidate(r.Context(), sessionID, path, value); err != nil {
		c.writeError(w, statusCodeForConfigError(err), err.Error())
		return
	}

	w.Header().Set("Content-Type", "application/json")
	c.writeJSON(w, OperationResponse{Status: "ok"})
}

func (c *Component) handleConfigSessionCommit(w http.ResponseWriter, r *http.Request) {
	sessionID := confhandler.SessionID(r.PathValue("session_id"))

	version, err := c.adapter.CommitCandidate(r.Context(), sessionID)
	if err != nil {
		c.writeError(w, statusCodeForConfigError(err), err.Error())
		return
	}

	w.Header().Set("Content-Type", "application/json")
	c.writeJSON(w, OperationResponse{
		Status:  "ok",
		Message: "Configuration committed successfully",
		Version: version,
	})
}

func (c *Component) handleConfigSessionDiscard(w http.ResponseWriter, r *http.Request) {
	sessionID := confhandler.SessionID(r.PathValue("session_id"))

	if err := c.adapter.DiscardCandidate(r.Context(), sessionID); err != nil {
		c.writeError(w, statusCodeForConfigError(err), err.Error())
		return
	}

	w.Header().Set("Content-Type", "application/json")
	c.writeJSON(w, OperationResponse{Status: "ok"})
}

func (c *Component) handleConfigSessionDiff(w http.ResponseWriter, r *http.Request) {
	sessionID := confhandler.SessionID(r.PathValue("session_id"))

	diff, err := c.adapter.DiffCandidate(r.Context(), sessionID)
	if err != nil {
		c.writeError(w, statusCodeForConfigError(err), err.Error())
		return
	}

	w.Header().Set("Content-Type", "application/json")
	c.writeJSON(w, convertDiffResult(diff))
}

func (c *Component) handleOpenAPISpec(w http.ResponseWriter, r *http.Request) {
	if c.specJSON == nil {
		c.writeError(w, http.StatusInternalServerError, "OpenAPI spec not available")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	if c.specETag != "" {
		w.Header().Set("ETag", c.specETag)
		if r.Header.Get("If-None-Match") == c.specETag {
			w.WriteHeader(http.StatusNotModified)
			return
		}
	}
	w.Write(c.specJSON)
}

func (c *Component) handleDocsRedirect(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, "/api/docs/", http.StatusMovedPermanently)
}

func (c *Component) writeShowResponse(w http.ResponseWriter, path string, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	c.writeJSON(w, ShowResponse{
		Path: path,
		Data: data,
	})
}

func decodeJSONBody(r *http.Request) (interface{}, error) {
	var value interface{}
	decoder := json.NewDecoder(r.Body)
	if err := decoder.Decode(&value); err != nil {
		return nil, err
	}

	return value, nil
}

func statusCodeForConfigError(err error) int {
	if err == nil {
		return http.StatusOK
	}

	switch {
	case strings.Contains(err.Error(), "configuration is locked by session"):
		return http.StatusConflict
	case strings.Contains(err.Error(), "session ") && strings.Contains(err.Error(), "not found"):
		return http.StatusNotFound
	case strings.Contains(err.Error(), "invalid version"):
		return http.StatusNotFound
	case strings.Contains(err.Error(), "failed to normalize path"):
		return http.StatusBadRequest
	case strings.Contains(err.Error(), "no changes to commit"):
		return http.StatusBadRequest
	default:
		return http.StatusInternalServerError
	}
}

func convertConfigVersions(versions []configmgr.ConfigVersion) []ConfigVersionResponse {
	result := make([]ConfigVersionResponse, 0, len(versions))
	for _, version := range versions {
		v := version
		result = append(result, convertConfigVersion(&v))
	}

	return result
}

func convertConfigVersion(version *configmgr.ConfigVersion) ConfigVersionResponse {
	if version == nil {
		return ConfigVersionResponse{}
	}

	result := ConfigVersionResponse{
		Version:   version.Version,
		Timestamp: version.Timestamp,
		CommitMsg: version.CommitMsg,
		Changes:   make([]ConfigChangeResponse, 0, len(version.Changes)),
	}

	for _, change := range version.Changes {
		result.Changes = append(result.Changes, ConfigChangeResponse{
			Type:  change.Type,
			Path:  change.Path,
			Value: stringifyChangeValue(change.Value),
		})
	}

	return result
}

func convertDiffResult(diff *configmgr.DiffResult) DiffResponse {
	if diff == nil {
		return DiffResponse{}
	}

	return DiffResponse{
		Added:    convertDiffLines(diff.Added),
		Deleted:  convertDiffLines(diff.Deleted),
		Modified: convertDiffLines(diff.Modified),
	}
}

func convertDiffLines(lines []configmgr.ConfigLine) []DiffLineResponse {
	result := make([]DiffLineResponse, 0, len(lines))
	for _, line := range lines {
		result = append(result, DiffLineResponse{
			Path:  line.Path,
			Value: line.Value,
		})
	}

	return result
}

func stringifyChangeValue(value interface{}) string {
	switch v := value.(type) {
	case string:
		return v
	default:
		data, err := json.Marshal(v)
		if err != nil {
			return ""
		}
		return string(data)
	}
}
