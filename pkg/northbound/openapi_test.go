// Copyright 2026 Veesix Networks Ltd
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package northbound

import (
	"context"
	"strings"
	"testing"

	"github.com/veesix-networks/osvbng/pkg/handlers/conf"
	confpaths "github.com/veesix-networks/osvbng/pkg/handlers/conf/paths"
	"github.com/veesix-networks/osvbng/pkg/handlers/oper"
	operpaths "github.com/veesix-networks/osvbng/pkg/handlers/oper/paths"
	"github.com/veesix-networks/osvbng/pkg/handlers/show"
	showpaths "github.com/veesix-networks/osvbng/pkg/handlers/show/paths"
)

func TestBuildOpenAPISpecIncludesTypedMetadata(t *testing.T) {
	showRegistry := show.NewRegistry()
	showRegistry.MustRegister(&testShowHandler{})

	confRegistry := conf.NewRegistry()
	confRegistry.MustRegister(&testConfHandler{})

	operRegistry := oper.NewRegistry()
	operRegistry.MustRegister(&testOperHandler{})

	spec, err := BuildOpenAPISpec(NewAdapter(showRegistry, confRegistry, operRegistry, nil))
	if err != nil {
		t.Fatalf("BuildOpenAPISpec() error = %v", err)
	}

	showPath := spec.Paths.Value("/api/show/subscriber/sessions")
	if showPath == nil || showPath.Get == nil {
		t.Fatalf("show path missing from OpenAPI spec")
	}

	var foundQuery bool
	for _, param := range showPath.Get.Parameters {
		if param.Value == nil || param.Value.Name != "access_type" {
			continue
		}
		foundQuery = true
		if param.Value.Schema == nil || param.Value.Schema.Value == nil {
			t.Fatalf("access_type query parameter schema missing")
		}
		if got := param.Value.Schema.Value.Default; got != "ipoe" {
			t.Fatalf("access_type default = %v, want ipoe", got)
		}
		if len(param.Value.Schema.Value.Enum) != 2 {
			t.Fatalf("access_type enum len = %d, want 2", len(param.Value.Schema.Value.Enum))
		}
	}
	if !foundQuery {
		t.Fatalf("access_type query parameter not generated")
	}

	operPath := spec.Paths.Value("/api/exec/system/events/debug")
	if operPath == nil || operPath.Post == nil || operPath.Post.RequestBody == nil || operPath.Post.RequestBody.Value == nil {
		t.Fatalf("oper path request body missing from OpenAPI spec")
	}

	operSchema := operPath.Post.RequestBody.Value.Content.Get("application/json").Schema
	if operSchema == nil || operSchema.Value == nil {
		t.Fatalf("oper request schema missing")
	}

	topics := operSchema.Value.Properties["topics"]
	if topics == nil || topics.Value == nil || topics.Value.Type == nil || !topics.Value.Type.Is("array") {
		t.Fatalf("topics property missing array schema")
	}

	profile := operSchema.Value.Properties["profile"]
	if profile == nil || profile.Value == nil {
		t.Fatalf("profile property missing")
	}
	if _, ok := profile.Value.Properties["role"]; !ok {
		t.Fatalf("nested profile.role property missing")
	}

	confPath := spec.Paths.Value("/api/set/example/hello/message")
	if confPath == nil || confPath.Post == nil || confPath.Post.RequestBody == nil || confPath.Post.RequestBody.Value == nil {
		t.Fatalf("config path request body missing from OpenAPI spec")
	}

	confSchema := confPath.Post.RequestBody.Value.Content.Get("application/json").Schema
	if confSchema == nil || confSchema.Value == nil || confSchema.Value.Type == nil || !confSchema.Value.Type.Is("string") {
		t.Fatalf("config ValueType() schema = %#v, want string", confSchema)
	}

	historyPath := spec.Paths.Value("/api/show/config/history")
	if historyPath == nil || historyPath.Get == nil {
		t.Fatalf("config history show path missing from OpenAPI spec")
	}

	sessionPath := spec.Paths.Value("/api/config/session/{session_id}/set/{path}")
	if sessionPath == nil || sessionPath.Post == nil {
		t.Fatalf("config session set path missing from OpenAPI spec")
	}
	if sessionPath.Post.RequestBody == nil || sessionPath.Post.RequestBody.Value == nil {
		t.Fatalf("config session set request body missing from OpenAPI spec")
	}

	var foundSessionID bool
	var foundPath bool
	for _, param := range sessionPath.Post.Parameters {
		if param.Value == nil {
			continue
		}
		switch param.Value.Name {
		case "session_id":
			foundSessionID = true
		case "path":
			foundPath = true
		}
	}
	if !foundSessionID || !foundPath {
		t.Fatalf("config session set parameters missing: session_id=%v path=%v", foundSessionID, foundPath)
	}

	runningAlias := spec.Paths.Value("/api/running-config")
	if runningAlias == nil || runningAlias.Get == nil || !runningAlias.Get.Deprecated {
		t.Fatalf("running-config alias should be present and deprecated")
	}
}

func TestBuildOpenAPISpecValidationFailsOnMissingMetadata(t *testing.T) {
	showRegistry := show.NewRegistry()
	confRegistry := conf.NewRegistry()
	confRegistry.MustRegister(&invalidConfHandler{})
	operRegistry := oper.NewRegistry()
	operRegistry.MustRegister(&invalidOperHandler{})

	_, err := BuildOpenAPISpec(NewAdapter(showRegistry, confRegistry, operRegistry, nil))
	if err == nil {
		t.Fatalf("BuildOpenAPISpec() error = nil, want validation failure")
	}

	message := err.Error()
	if !strings.Contains(message, `oper handler "system.logging.level.<*>" missing InputType()`) {
		t.Fatalf("validation error missing oper detail: %v", err)
	}
	if !strings.Contains(message, `config handler "example.plugin.setting" requires ValueType()`) {
		t.Fatalf("validation error missing config detail: %v", err)
	}
}

type testShowHandler struct{}

type testShowOptions struct {
	AccessType string `query:"access_type" description:"Access type filter" default:"ipoe" enum:"ipoe,pppoe"`
}

type testShowResponse struct {
	ID string `json:"id"`
}

func (h *testShowHandler) Collect(context.Context, *show.Request) (interface{}, error) {
	return []testShowResponse{}, nil
}

func (h *testShowHandler) PathPattern() showpaths.Path {
	return showpaths.Path("subscriber.sessions")
}

func (h *testShowHandler) Dependencies() []showpaths.Path {
	return nil
}

func (h *testShowHandler) OptionsType() interface{} {
	return &testShowOptions{}
}

func (h *testShowHandler) OutputType() interface{} {
	return []testShowResponse{}
}

type testConfHandler struct{}

func (h *testConfHandler) Validate(context.Context, *conf.HandlerContext) error {
	return nil
}

func (h *testConfHandler) Apply(context.Context, *conf.HandlerContext) error {
	return nil
}

func (h *testConfHandler) Rollback(context.Context, *conf.HandlerContext) error {
	return nil
}

func (h *testConfHandler) PathPattern() confpaths.Path {
	return confpaths.Path("example.hello.message")
}

func (h *testConfHandler) Dependencies() []confpaths.Path {
	return nil
}

func (h *testConfHandler) Callbacks() *conf.Callbacks {
	return nil
}

func (h *testConfHandler) ValueType() interface{} {
	return ""
}

type testOperHandler struct{}

type testOperRequest struct {
	Topics  []string        `json:"topics"`
	Profile testOperProfile `json:"profile"`
}

type testOperProfile struct {
	Role string `json:"role"`
}

type testOperResponse struct {
	Success bool `json:"success"`
}

func (h *testOperHandler) Execute(context.Context, *oper.Request) (interface{}, error) {
	return &testOperResponse{Success: true}, nil
}

func (h *testOperHandler) PathPattern() operpaths.Path {
	return operpaths.Path("system.events.debug")
}

func (h *testOperHandler) Dependencies() []operpaths.Path {
	return nil
}

func (h *testOperHandler) InputType() interface{} {
	return &testOperRequest{}
}

func (h *testOperHandler) OutputType() interface{} {
	return &testOperResponse{}
}

type invalidConfHandler struct{}

func (h *invalidConfHandler) Validate(context.Context, *conf.HandlerContext) error {
	return nil
}

func (h *invalidConfHandler) Apply(context.Context, *conf.HandlerContext) error {
	return nil
}

func (h *invalidConfHandler) Rollback(context.Context, *conf.HandlerContext) error {
	return nil
}

func (h *invalidConfHandler) PathPattern() confpaths.Path {
	return confpaths.Path("example.plugin.setting")
}

func (h *invalidConfHandler) Dependencies() []confpaths.Path {
	return nil
}

func (h *invalidConfHandler) Callbacks() *conf.Callbacks {
	return nil
}

type invalidOperHandler struct{}

func (h *invalidOperHandler) Execute(context.Context, *oper.Request) (interface{}, error) {
	return nil, nil
}

func (h *invalidOperHandler) PathPattern() operpaths.Path {
	return operpaths.Path("system.logging.level.<*>")
}

func (h *invalidOperHandler) Dependencies() []operpaths.Path {
	return nil
}
