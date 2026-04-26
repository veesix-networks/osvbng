package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/veesix-networks/osvbng/pkg/configmgr"
	"github.com/veesix-networks/osvbng/pkg/handlers/conf"
	confpaths "github.com/veesix-networks/osvbng/pkg/handlers/conf/paths"
	"github.com/veesix-networks/osvbng/pkg/handlers/oper"
	"github.com/veesix-networks/osvbng/pkg/handlers/show"
	showpaths "github.com/veesix-networks/osvbng/pkg/handlers/show/paths"
	"github.com/veesix-networks/osvbng/pkg/logger"
	"github.com/veesix-networks/osvbng/pkg/northbound"
)

type testConfigLeafHandler struct {
	path confpaths.Path
}

func (h *testConfigLeafHandler) Validate(context.Context, *conf.HandlerContext) error { return nil }
func (h *testConfigLeafHandler) Apply(context.Context, *conf.HandlerContext) error    { return nil }
func (h *testConfigLeafHandler) Rollback(context.Context, *conf.HandlerContext) error { return nil }
func (h *testConfigLeafHandler) PathPattern() confpaths.Path                          { return h.path }
func (h *testConfigLeafHandler) Dependencies() []confpaths.Path                       { return nil }
func (h *testConfigLeafHandler) Callbacks() *conf.Callbacks                           { return nil }

func newTestComponent(t *testing.T) (*Component, http.Handler) {
	t.Helper()

	configd := configmgr.NewConfigManager()
	configd.GetRegistry().MustRegister(&testConfigLeafHandler{path: "interfaces.<*>.enabled"})

	adapter := northbound.NewAdapter(show.NewRegistry(), configd.GetRegistry(), oper.NewRegistry(), configd)

	component := &Component{
		logger:   logger.Get(Namespace),
		adapter:  adapter,
		specJSON: []byte(`{"openapi":"3.0.3"}`),
		specETag: `"test-etag"`,
	}

	return component, component.newMux()
}

func TestConfigSessionSetDiffAndDiscard(t *testing.T) {
	_, mux := newTestComponent(t)

	createReq := httptest.NewRequest(http.MethodPost, "/api/config/session", nil)
	createRec := httptest.NewRecorder()
	mux.ServeHTTP(createRec, createReq)
	if createRec.Code != http.StatusCreated {
		t.Fatalf("create session status = %d, want %d", createRec.Code, http.StatusCreated)
	}

	var created ConfigSessionCreateResponse
	if err := json.Unmarshal(createRec.Body.Bytes(), &created); err != nil {
		t.Fatalf("unmarshal create response: %v", err)
	}
	if created.SessionID == "" {
		t.Fatal("session_id should not be empty")
	}

	setReq := httptest.NewRequest(http.MethodPost, "/api/config/session/"+created.SessionID+"/set/interfaces/eth0/enabled", bytes.NewBufferString("true"))
	setRec := httptest.NewRecorder()
	mux.ServeHTTP(setRec, setReq)
	if setRec.Code != http.StatusOK {
		t.Fatalf("set session status = %d, want %d, body = %s", setRec.Code, http.StatusOK, setRec.Body.String())
	}

	diffReq := httptest.NewRequest(http.MethodGet, "/api/config/session/"+created.SessionID+"/diff", nil)
	diffRec := httptest.NewRecorder()
	mux.ServeHTTP(diffRec, diffReq)
	if diffRec.Code != http.StatusOK {
		t.Fatalf("diff session status = %d, want %d", diffRec.Code, http.StatusOK)
	}

	var diff DiffResponse
	if err := json.Unmarshal(diffRec.Body.Bytes(), &diff); err != nil {
		t.Fatalf("unmarshal diff response: %v", err)
	}
	if len(diff.Added) != 1 {
		t.Fatalf("diff added len = %d, want 1", len(diff.Added))
	}
	if diff.Added[0].Path != "interfaces.eth0.enabled" {
		t.Fatalf("diff path = %q, want interfaces.eth0.enabled", diff.Added[0].Path)
	}
	if diff.Added[0].Value != "true" {
		t.Fatalf("diff value = %q, want true", diff.Added[0].Value)
	}

	discardReq := httptest.NewRequest(http.MethodPost, "/api/config/session/"+created.SessionID+"/discard", nil)
	discardRec := httptest.NewRecorder()
	mux.ServeHTTP(discardRec, discardReq)
	if discardRec.Code != http.StatusOK {
		t.Fatalf("discard session status = %d, want %d", discardRec.Code, http.StatusOK)
	}

	missingDiffReq := httptest.NewRequest(http.MethodGet, "/api/config/session/"+created.SessionID+"/diff", nil)
	missingDiffRec := httptest.NewRecorder()
	mux.ServeHTTP(missingDiffRec, missingDiffReq)
	if missingDiffRec.Code != http.StatusNotFound {
		t.Fatalf("expired diff status = %d, want %d", missingDiffRec.Code, http.StatusNotFound)
	}
}

func TestConfigSessionCreateRespectsLock(t *testing.T) {
	_, mux := newTestComponent(t)

	firstReq := httptest.NewRequest(http.MethodPost, "/api/config/session", nil)
	firstRec := httptest.NewRecorder()
	mux.ServeHTTP(firstRec, firstReq)
	if firstRec.Code != http.StatusCreated {
		t.Fatalf("first create session status = %d, want %d", firstRec.Code, http.StatusCreated)
	}

	secondReq := httptest.NewRequest(http.MethodPost, "/api/config/session", nil)
	secondRec := httptest.NewRecorder()
	mux.ServeHTTP(secondRec, secondReq)
	if secondRec.Code != http.StatusConflict {
		t.Fatalf("second create session status = %d, want %d body=%s", secondRec.Code, http.StatusConflict, secondRec.Body.String())
	}
}

type fakeItem struct {
	ID    string `json:"id"`
	Score int    `json:"score"`
}

type fakeShowHandler struct {
	path    showpaths.Path
	data    interface{}
	sortKey string
}

func (h *fakeShowHandler) Collect(_ context.Context, _ *show.Request) (interface{}, error) {
	return h.data, nil
}
func (h *fakeShowHandler) PathPattern() showpaths.Path     { return h.path }
func (h *fakeShowHandler) Dependencies() []showpaths.Path  { return nil }
func (h *fakeShowHandler) Summary() string                 { return "fake" }
func (h *fakeShowHandler) SortKey() string                 { return h.sortKey }

func newTestComponentWithShowHandler(t *testing.T, handler show.ShowHandler) (*Component, http.Handler) {
	t.Helper()
	configd := configmgr.NewConfigManager()
	configd.GetRegistry().MustRegister(&testConfigLeafHandler{path: "interfaces.<*>.enabled"})

	showReg := show.NewRegistry()
	showReg.MustRegister(handler)

	adapter := northbound.NewAdapter(showReg, configd.GetRegistry(), oper.NewRegistry(), configd)

	component := &Component{
		logger:   logger.Get(Namespace),
		adapter:  adapter,
		specJSON: []byte(`{"openapi":"3.0.3"}`),
		specETag: `"test-etag"`,
	}
	return component, component.newMux()
}

func TestPaginatedShowEndpoint_DefaultLimit(t *testing.T) {
	items := make([]fakeItem, 250)
	for i := range items {
		items[i] = fakeItem{ID: fmt.Sprintf("id-%03d", i), Score: i}
	}
	_, mux := newTestComponentWithShowHandler(t, &fakeShowHandler{
		path:    "fake.list",
		data:    items,
		sortKey: "id",
	})

	req := httptest.NewRequest(http.MethodGet, "/api/show/fake/list", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body = %s", rec.Code, rec.Body.String())
	}

	var resp PaginatedShowResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.Path != "fake.list" {
		t.Fatalf("path = %q", resp.Path)
	}
	if resp.Pagination.Total != 250 {
		t.Fatalf("total = %d, want 250", resp.Pagination.Total)
	}
	if resp.Pagination.Limit != 100 {
		t.Fatalf("limit = %d, want 100", resp.Pagination.Limit)
	}
	if resp.Pagination.Offset != 0 {
		t.Fatalf("offset = %d, want 0", resp.Pagination.Offset)
	}
	if resp.Pagination.Returned != 100 {
		t.Fatalf("returned = %d, want 100", resp.Pagination.Returned)
	}
	if !resp.Pagination.HasMore {
		t.Fatal("has_more should be true")
	}
}

func TestPaginatedShowEndpoint_OffsetLastPage(t *testing.T) {
	items := make([]fakeItem, 250)
	for i := range items {
		items[i] = fakeItem{ID: fmt.Sprintf("id-%03d", i), Score: i}
	}
	_, mux := newTestComponentWithShowHandler(t, &fakeShowHandler{
		path:    "fake.list",
		data:    items,
		sortKey: "id",
	})

	req := httptest.NewRequest(http.MethodGet, "/api/show/fake/list?limit=100&offset=200", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	var resp PaginatedShowResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.Pagination.Returned != 50 {
		t.Fatalf("returned = %d, want 50", resp.Pagination.Returned)
	}
	if resp.Pagination.HasMore {
		t.Fatal("has_more should be false on last page")
	}
}

func TestPaginatedShowEndpoint_LimitClampedToMax(t *testing.T) {
	items := make([]fakeItem, 2000)
	for i := range items {
		items[i] = fakeItem{ID: fmt.Sprintf("id-%04d", i)}
	}
	_, mux := newTestComponentWithShowHandler(t, &fakeShowHandler{
		path:    "fake.list",
		data:    items,
		sortKey: "id",
	})

	req := httptest.NewRequest(http.MethodGet, "/api/show/fake/list?limit=50000", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	var resp PaginatedShowResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.Pagination.Limit != 1000 {
		t.Fatalf("limit = %d, want clamp to 1000", resp.Pagination.Limit)
	}
	if resp.Pagination.Returned != 1000 {
		t.Fatalf("returned = %d, want 1000", resp.Pagination.Returned)
	}
}

func TestPaginatedShowEndpoint_StableOrderAcrossCalls(t *testing.T) {
	items := []fakeItem{
		{ID: "z"}, {ID: "a"}, {ID: "m"}, {ID: "c"}, {ID: "b"},
	}
	_, mux := newTestComponentWithShowHandler(t, &fakeShowHandler{
		path:    "fake.list",
		data:    items,
		sortKey: "id",
	})

	doCall := func() []fakeItem {
		req := httptest.NewRequest(http.MethodGet, "/api/show/fake/list?limit=10", nil)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		var resp struct {
			Data []fakeItem `json:"data"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		return resp.Data
	}

	first := doCall()
	second := doCall()
	if len(first) != 5 || len(second) != 5 {
		t.Fatalf("len mismatch: %d vs %d", len(first), len(second))
	}
	expected := []string{"a", "b", "c", "m", "z"}
	for i, item := range first {
		if item.ID != expected[i] {
			t.Fatalf("first call sort: pos %d got %q want %q", i, item.ID, expected[i])
		}
		if second[i].ID != expected[i] {
			t.Fatalf("second call sort: pos %d got %q want %q", i, second[i].ID, expected[i])
		}
	}
}

func TestPaginatedShowEndpoint_PassThroughForStruct(t *testing.T) {
	type stats struct {
		Total int `json:"total"`
	}
	_, mux := newTestComponentWithShowHandler(t, &fakeShowHandler{
		path: "fake.stats",
		data: stats{Total: 42},
	})

	req := httptest.NewRequest(http.MethodGet, "/api/show/fake/stats", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	var resp ShowResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.Path != "fake.stats" {
		t.Fatalf("path = %q", resp.Path)
	}
	if rec.Body.String() == "" {
		t.Fatal("body empty")
	}
	if bytes.Contains(rec.Body.Bytes(), []byte(`"pagination"`)) {
		t.Fatalf("non-list response should not include pagination block: %s", rec.Body.String())
	}
}

func TestCanonicalShowConfigRoutes(t *testing.T) {
	_, mux := newTestComponent(t)

	runningReq := httptest.NewRequest(http.MethodGet, "/api/show/running-config", nil)
	runningRec := httptest.NewRecorder()
	mux.ServeHTTP(runningRec, runningReq)
	if runningRec.Code != http.StatusOK {
		t.Fatalf("show running-config status = %d, want %d", runningRec.Code, http.StatusOK)
	}

	var running ShowResponse
	if err := json.Unmarshal(runningRec.Body.Bytes(), &running); err != nil {
		t.Fatalf("unmarshal running-config response: %v", err)
	}
	if running.Path != "running-config" {
		t.Fatalf("running-config path = %q, want running-config", running.Path)
	}

	historyReq := httptest.NewRequest(http.MethodGet, "/api/show/config/history", nil)
	historyRec := httptest.NewRecorder()
	mux.ServeHTTP(historyRec, historyReq)
	if historyRec.Code != http.StatusOK {
		t.Fatalf("show config history status = %d, want %d", historyRec.Code, http.StatusOK)
	}

	var history ShowResponse
	if err := json.Unmarshal(historyRec.Body.Bytes(), &history); err != nil {
		t.Fatalf("unmarshal config history response: %v", err)
	}
	if history.Path != "config.history" {
		t.Fatalf("config history path = %q, want config.history", history.Path)
	}

	versionReq := httptest.NewRequest(http.MethodGet, "/api/show/config/version/1", nil)
	versionRec := httptest.NewRecorder()
	mux.ServeHTTP(versionRec, versionReq)
	if versionRec.Code != http.StatusNotFound {
		t.Fatalf("show config version status = %d, want %d", versionRec.Code, http.StatusNotFound)
	}
}
