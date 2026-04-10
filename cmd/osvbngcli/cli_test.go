package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
)

func TestBuildContractFlattensNestedSetBodyAndQueryFlags(t *testing.T) {
	spec := &openapi3.T{
		OpenAPI: "3.0.3",
		Paths:   openapi3.NewPaths(),
	}

	spec.Paths.Set("/api/show/subscriber/sessions", &openapi3.PathItem{
		Get: &openapi3.Operation{
			Parameters: openapi3.Parameters{
				&openapi3.ParameterRef{Value: &openapi3.Parameter{
					Name:     "protocol",
					In:       "query",
					Required: false,
					Schema:   &openapi3.SchemaRef{Value: &openapi3.Schema{Type: &openapi3.Types{"string"}, Enum: []interface{}{"dhcpv4", "dhcpv6"}}},
				}},
			},
		},
	})

	spec.Paths.Set("/api/set/interfaces/{name}", &openapi3.PathItem{
		Post: &openapi3.Operation{
			Parameters: openapi3.Parameters{
				&openapi3.ParameterRef{Value: &openapi3.Parameter{
					Name:     "name",
					In:       "path",
					Required: true,
					Schema:   &openapi3.SchemaRef{Value: &openapi3.Schema{Type: &openapi3.Types{"string"}}},
				}},
			},
			RequestBody: &openapi3.RequestBodyRef{
				Value: &openapi3.RequestBody{
					Required: true,
					Content: openapi3.NewContentWithJSONSchemaRef(&openapi3.SchemaRef{Value: &openapi3.Schema{
						Type: &openapi3.Types{"object"},
						Properties: openapi3.Schemas{
							"description": {Value: &openapi3.Schema{Type: &openapi3.Types{"string"}}},
							"address": {Value: &openapi3.Schema{
								Type: &openapi3.Types{"object"},
								Properties: openapi3.Schemas{
									"ipv4": {Value: &openapi3.Schema{
										Type:  &openapi3.Types{"array"},
										Items: &openapi3.SchemaRef{Value: &openapi3.Schema{Type: &openapi3.Types{"string"}}},
									}},
								},
							}},
						},
					}}),
				},
			},
		},
	})

	contract, err := buildContract(spec)
	if err != nil {
		t.Fatalf("buildContract() error = %v", err)
	}

	var showCommand *GeneratedCommand
	var setCommand *GeneratedCommand
	for _, command := range contract.Commands {
		switch strings.Join(commandPathTokens(command), " ") {
		case "show subscriber sessions":
			showCommand = command
		case "set interfaces <name>":
			setCommand = command
		}
	}

	if showCommand == nil {
		t.Fatal("show command missing from contract")
	}
	if len(showCommand.QueryFlags) != 1 || showCommand.QueryFlags[0].CLIName != "protocol" {
		t.Fatalf("show query flags = %#v, want protocol", showCommand.QueryFlags)
	}

	if setCommand == nil {
		t.Fatal("set command missing from contract")
	}
	if setCommand.Body.Mode != BodyModeFlattened {
		t.Fatalf("set body mode = %v, want flattened", setCommand.Body.Mode)
	}

	gotFlags := make([]string, 0, len(setCommand.Body.Flags))
	for _, flag := range setCommand.Body.Flags {
		gotFlags = append(gotFlags, flag.CLIName)
	}
	if !reflect.DeepEqual(gotFlags, []string{"address.ipv4", "description"}) {
		t.Fatalf("set body flags = %v, want [address.ipv4 description]", gotFlags)
	}
	if !setCommand.Body.Flags[0].Repeated {
		t.Fatalf("address.ipv4 should be repeated")
	}
}

func TestCLIProcessCommandUsesGeneratedContractAndConfigSession(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())

	specJSON := mustSpecJSON(t)

	var (
		gotShowProtocol string
		gotExecBody     map[string]interface{}
		gotImmediateSet string
		gotSessionSet   string
	)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/openapi.json":
			w.Header().Set("ETag", `"test-etag"`)
			_, _ = w.Write(specJSON)
		case r.Method == http.MethodGet && r.URL.Path == "/api/show/subscriber/sessions":
			gotShowProtocol = r.URL.Query().Get("protocol")
			writeJSON(t, w, map[string]interface{}{
				"path": "subscriber.sessions",
				"data": []map[string]interface{}{{"session_id": "session-1"}},
			})
		case r.Method == http.MethodPost && r.URL.Path == "/api/exec/system/events/debug":
			mustDecodeJSON(t, r.Body, &gotExecBody)
			writeJSON(t, w, map[string]interface{}{"ok": true})
		case r.Method == http.MethodPost && r.URL.Path == "/api/config/session":
			w.WriteHeader(http.StatusCreated)
			writeJSON(t, w, map[string]string{"session_id": "session-1"})
		case r.Method == http.MethodPost && r.URL.Path == "/api/config/session/session-1/set/example/hello/message":
			mustDecodeJSON(t, r.Body, &gotSessionSet)
			writeJSON(t, w, map[string]string{"status": "ok"})
		case r.Method == http.MethodPost && r.URL.Path == "/api/config/session/session-1/commit":
			writeJSON(t, w, map[string]interface{}{"status": "ok", "version": 2})
		case r.Method == http.MethodPost && r.URL.Path == "/api/set/example/hello/message":
			mustDecodeJSON(t, r.Body, &gotImmediateSet)
			writeJSON(t, w, map[string]string{"status": "ok"})
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()

	cli, err := NewCLI(server.URL)
	if err != nil {
		t.Fatalf("NewCLI() error = %v", err)
	}

	output := captureStdout(t, func() {
		if err := cli.processCommand("show subscriber sessions --protocol dhcpv4 | json"); err != nil {
			t.Fatalf("show command error = %v", err)
		}
	})
	if gotShowProtocol != "dhcpv4" {
		t.Fatalf("show protocol query = %q, want dhcpv4", gotShowProtocol)
	}
	if !strings.Contains(output, `"session_id": "session-1"`) {
		t.Fatalf("show output = %q, want JSON data", output)
	}

	if err := cli.processCommand("exec system events debug --topics ha --topics subscriber"); err != nil {
		t.Fatalf("exec command error = %v", err)
	}
	gotTopics, ok := gotExecBody["topics"].([]interface{})
	if !ok || len(gotTopics) != 2 || gotTopics[0] != "ha" || gotTopics[1] != "subscriber" {
		t.Fatalf("exec topics body = %#v, want [ha subscriber]", gotExecBody["topics"])
	}

	if err := cli.processCommand("configure"); err != nil {
		t.Fatalf("configure error = %v", err)
	}
	if !cli.configMode || cli.configSessionID != "session-1" {
		t.Fatalf("config mode state = (%v, %q), want (true, session-1)", cli.configMode, cli.configSessionID)
	}

	if err := cli.processCommand("set example hello message --value hello"); err != nil {
		t.Fatalf("session set command error = %v", err)
	}
	if gotSessionSet != "hello" {
		t.Fatalf("session set body = %q, want hello", gotSessionSet)
	}

	if err := cli.processCommand("commit"); err != nil {
		t.Fatalf("commit error = %v", err)
	}
	if cli.configMode || cli.configSessionID != "" {
		t.Fatalf("config mode after commit = (%v, %q), want (false, \"\")", cli.configMode, cli.configSessionID)
	}

	if err := cli.processCommand("set example hello message --value world"); err != nil {
		t.Fatalf("immediate set command error = %v", err)
	}
	if gotImmediateSet != "world" {
		t.Fatalf("immediate set body = %q, want world", gotImmediateSet)
	}
}

func TestCLIProcessCommandAcceptsPositionalScalarValue(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())

	specJSON := mustSpecJSON(t)
	var gotImmediateSet string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/openapi.json":
			_, _ = w.Write(specJSON)
		case r.Method == http.MethodPost && r.URL.Path == "/api/set/example/hello/message":
			mustDecodeJSON(t, r.Body, &gotImmediateSet)
			writeJSON(t, w, map[string]string{"status": "ok"})
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()

	cli, err := NewCLI(server.URL)
	if err != nil {
		t.Fatalf("NewCLI() error = %v", err)
	}

	if err := cli.processCommand("set example hello message hello"); err != nil {
		t.Fatalf("processCommand() error = %v", err)
	}
	if gotImmediateSet != "hello" {
		t.Fatalf("immediate set body = %q, want hello", gotImmediateSet)
	}
}

func TestProcessCommandQuestionMarkShowsNextLevelSuggestions(t *testing.T) {
	spec := &openapi3.T{
		OpenAPI: "3.0.3",
		Paths:   openapi3.NewPaths(),
	}

	spec.Paths.Set("/api/show/interfaces", &openapi3.PathItem{
		Get: &openapi3.Operation{
			Summary: "show interfaces",
		},
	})
	spec.Paths.Set("/api/show/subscriber/sessions", &openapi3.PathItem{
		Get: &openapi3.Operation{
			Summary: "show subscriber sessions",
		},
	})
	spec.Paths.Set("/api/show/system/cache-keys", &openapi3.PathItem{
		Get: &openapi3.Operation{
			Summary: "show system cache keys",
		},
	})

	cli := newTestCLIFromSpec(t, spec)

	output := captureStdout(t, func() {
		if err := cli.processCommand("show ?"); err != nil {
			t.Fatalf("processCommand(show ?) error = %v", err)
		}
	})

	for _, expected := range []string{"interfaces", "subscriber", "system"} {
		if !strings.Contains(output, expected) {
			t.Fatalf("question-mark output = %q, want %q", output, expected)
		}
	}
	if strings.Contains(output, "\n  show") {
		t.Fatalf("question-mark output = %q, should list next-level suggestions instead of literal show", output)
	}
}

func TestProcessCommandQuestionMarkShowsScalarValuePlaceholder(t *testing.T) {
	cli := newTestCLIFromSpec(t, mustSpec(t))

	output := captureStdout(t, func() {
		if err := cli.processCommand("set example hello message ?"); err != nil {
			t.Fatalf("processCommand(set example hello message ?) error = %v", err)
		}
	})

	if !strings.Contains(output, "<value>") {
		t.Fatalf("question-mark output = %q, want positional <value> placeholder", output)
	}
	if strings.Contains(output, "--value") {
		t.Fatalf("question-mark output = %q, should not require --value for top-level scalar set commands", output)
	}
}

func TestProcessCommandQuestionMarkOnExactCommandShowsFlags(t *testing.T) {
	cli := newTestCLIFromSpec(t, mustSpec(t))

	output := captureStdout(t, func() {
		if err := cli.processCommand("show subscriber sessions ?"); err != nil {
			t.Fatalf("processCommand(show subscriber sessions ?) error = %v", err)
		}
	})

	if !strings.Contains(output, "--protocol") {
		t.Fatalf("question-mark output = %q, want --protocol flag suggestion", output)
	}
	if strings.Contains(output, "Usage: show subscriber sessions") {
		t.Fatalf("question-mark output = %q, should show flag suggestions instead of command help", output)
	}
}

func mustSpecJSON(t *testing.T) []byte {
	t.Helper()

	spec := mustSpec(t)

	data, err := json.Marshal(spec)
	if err != nil {
		t.Fatalf("json.Marshal(spec) error = %v", err)
	}
	return data
}

func mustSpec(t *testing.T) *openapi3.T {
	t.Helper()

	spec := &openapi3.T{
		OpenAPI: "3.0.3",
		Paths:   openapi3.NewPaths(),
	}

	spec.Paths.Set("/api/show/subscriber/sessions", &openapi3.PathItem{
		Get: &openapi3.Operation{
			Parameters: openapi3.Parameters{
				&openapi3.ParameterRef{Value: &openapi3.Parameter{
					Name:     "protocol",
					In:       "query",
					Schema:   &openapi3.SchemaRef{Value: &openapi3.Schema{Type: &openapi3.Types{"string"}}},
					Required: false,
				}},
			},
		},
	})

	spec.Paths.Set("/api/exec/system/events/debug", &openapi3.PathItem{
		Post: &openapi3.Operation{
			RequestBody: &openapi3.RequestBodyRef{
				Value: &openapi3.RequestBody{
					Content: openapi3.NewContentWithJSONSchemaRef(&openapi3.SchemaRef{Value: &openapi3.Schema{
						Type: &openapi3.Types{"object"},
						Properties: openapi3.Schemas{
							"topics": {Value: &openapi3.Schema{
								Type:  &openapi3.Types{"array"},
								Items: &openapi3.SchemaRef{Value: &openapi3.Schema{Type: &openapi3.Types{"string"}}},
							}},
						},
					}}),
				},
			},
		},
	})

	spec.Paths.Set("/api/set/example/hello/message", &openapi3.PathItem{
		Post: &openapi3.Operation{
			RequestBody: &openapi3.RequestBodyRef{
				Value: &openapi3.RequestBody{
					Required: true,
					Content:  openapi3.NewContentWithJSONSchemaRef(&openapi3.SchemaRef{Value: &openapi3.Schema{Type: &openapi3.Types{"string"}}}),
				},
			},
		},
	})

	return spec
}

func newTestCLIFromSpec(t *testing.T, spec *openapi3.T) *CLI {
	t.Helper()

	contract, err := buildContract(spec)
	if err != nil {
		t.Fatalf("buildContract() error = %v", err)
	}
	return &CLI{
		formatter: NewGenericFormatter(),
		contract:  contract,
	}
}

func mustDecodeJSON(t *testing.T, body io.Reader, out interface{}) {
	t.Helper()

	if err := json.NewDecoder(body).Decode(out); err != nil {
		t.Fatalf("decode JSON body: %v", err)
	}
}

func writeJSON(t *testing.T, w http.ResponseWriter, value interface{}) {
	t.Helper()

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(value); err != nil {
		t.Fatalf("encode JSON response: %v", err)
	}
}

func captureStdout(t *testing.T, fn func()) string {
	t.Helper()

	original := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe() error = %v", err)
	}
	os.Stdout = w

	outputCh := make(chan string, 1)
	go func() {
		var buf bytes.Buffer
		_, _ = io.Copy(&buf, r)
		outputCh <- buf.String()
	}()

	fn()

	_ = w.Close()
	os.Stdout = original
	return <-outputCh
}

func commandPathTokens(command *GeneratedCommand) []string {
	parts := make([]string, 0, len(command.Segments))
	for _, segment := range command.Segments {
		if segment.Param != nil {
			parts = append(parts, formatPlaceholder(segment.Param.Name))
			continue
		}
		parts = append(parts, segment.Literal)
	}
	return parts
}

func TestTokenizeLineSupportsQuotedValues(t *testing.T) {
	tokens, err := tokenizeLine(`set foo --json '{"hello":"world test"}'`, false)
	if err != nil {
		t.Fatalf("tokenizeLine() error = %v", err)
	}

	want := []string{"set", "foo", "--json", `{"hello":"world test"}`}
	if !reflect.DeepEqual(tokens, want) {
		t.Fatalf("tokens = %#v, want %#v", tokens, want)
	}
}

func TestBestEffortDiscardClearsConfigModeOnFailure(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/openapi.json":
			_, _ = w.Write(mustSpecJSON(t))
		case r.Method == http.MethodPost && r.URL.Path == "/api/config/session/session-1/discard":
			w.WriteHeader(http.StatusNotFound)
			writeJSON(t, w, map[string]string{"error": "session session-1 not found"})
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()

	cli, err := NewCLI(server.URL)
	if err != nil {
		t.Fatalf("NewCLI() error = %v", err)
	}
	cli.configMode = true
	cli.configSessionID = "session-1"

	err = cli.bestEffortDiscard()
	if err == nil || !strings.Contains(err.Error(), "session session-1 not found") {
		t.Fatalf("bestEffortDiscard() error = %v, want not found", err)
	}
	if cli.configMode || cli.configSessionID != "" {
		t.Fatalf("config mode after bestEffortDiscard = (%v, %q), want (false, \"\")", cli.configMode, cli.configSessionID)
	}
}

func TestEnterConfigModeReturnsHelpfulMessageWhenServerLacksEndpoint(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/openapi.json":
			_, _ = w.Write(mustSpecJSON(t))
		case r.Method == http.MethodPost && r.URL.Path == "/api/config/session":
			http.NotFound(w, r)
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()

	cli, err := NewCLI(server.URL)
	if err != nil {
		t.Fatalf("NewCLI() error = %v", err)
	}

	err = cli.processCommand("configure")
	if err == nil || !strings.Contains(err.Error(), "server does not support /api/config/session") {
		t.Fatalf("configure error = %v, want unsupported config-session message", err)
	}
}

func TestLoadContractUsesCached304(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())

	var requestCount int
	specJSON := mustSpecJSON(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		if requestCount == 1 {
			w.Header().Set("ETag", `"etag-1"`)
			_, _ = w.Write(specJSON)
			return
		}
		if got := r.Header.Get("If-None-Match"); got != `"etag-1"` {
			t.Fatalf("If-None-Match = %q, want %q", got, `"etag-1"`)
		}
		w.WriteHeader(http.StatusNotModified)
	}))
	defer server.Close()

	client, err := newAPIClient(server.URL)
	if err != nil {
		t.Fatalf("newAPIClient() error = %v", err)
	}

	ctx := context.Background()
	if _, err := client.loadContract(ctx); err != nil {
		t.Fatalf("first loadContract() error = %v", err)
	}
	if _, err := client.loadContract(ctx); err != nil {
		t.Fatalf("second loadContract() error = %v", err)
	}
	if requestCount != 2 {
		t.Fatalf("requestCount = %d, want 2", requestCount)
	}
}
