// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package northbound

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"reflect"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/veesix-networks/osvbng/pkg/config"
	"github.com/veesix-networks/osvbng/pkg/handlers/conf"
	"github.com/veesix-networks/osvbng/pkg/handlers/oper"
	"github.com/veesix-networks/osvbng/pkg/handlers/show"
	"github.com/veesix-networks/osvbng/pkg/version"
)

var wildcardRe = regexp.MustCompile(`<([^>]+)>`)

type OpenAPISpecData struct {
	Spec *openapi3.T
	JSON []byte
	ETag string
}

type pathsResponse struct {
	ShowPaths   []string `json:"show_paths"`
	ConfigPaths []string `json:"config_paths"`
	OperPaths   []string `json:"oper_paths"`
}

type showResponse struct {
	Path string      `json:"path"`
	Data interface{} `json:"data"`
}

type errorResponse struct {
	Error string `json:"error"`
}

type statusResponse struct {
	Status string `json:"status"`
}

type actionResponse struct {
	Status  string `json:"status"`
	Message string `json:"message,omitempty"`
	Version int    `json:"version,omitempty"`
}

type configSessionCreateResponse struct {
	SessionID string `json:"session_id"`
}

type configHistoryResponse struct {
	Versions []configVersionResponse `json:"versions"`
}

type configVersionResponse struct {
	Version   int                    `json:"version"`
	Timestamp time.Time              `json:"timestamp"`
	CommitMsg string                 `json:"commit_msg,omitempty"`
	Changes   []configChangeResponse `json:"changes,omitempty"`
}

type configChangeResponse struct {
	Type  string `json:"type"`
	Path  string `json:"path"`
	Value string `json:"value,omitempty"`
}

type diffResponse struct {
	Added    []diffLineResponse `json:"added,omitempty"`
	Deleted  []diffLineResponse `json:"deleted,omitempty"`
	Modified []diffLineResponse `json:"modified,omitempty"`
}

type diffLineResponse struct {
	Path  string `json:"path"`
	Value string `json:"value,omitempty"`
}

func GenerateOpenAPISpec(adapter *Adapter) (*OpenAPISpecData, error) {
	spec, err := BuildOpenAPISpec(adapter)
	if err != nil {
		return nil, err
	}

	specJSON, err := json.MarshalIndent(spec, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshal OpenAPI spec: %w", err)
	}

	sum := sha256.Sum256(specJSON)

	return &OpenAPISpecData{
		Spec: spec,
		JSON: specJSON,
		ETag: `"` + hex.EncodeToString(sum[:]) + `"`,
	}, nil
}

func BuildOpenAPISpec(adapter *Adapter) (*openapi3.T, error) {
	if adapter == nil {
		return nil, fmt.Errorf("northbound adapter is required")
	}

	if err := validateOpenAPIMetadata(adapter); err != nil {
		return nil, err
	}

	tagSet := map[string]bool{"General": true}

	spec := &openapi3.T{
		OpenAPI: "3.0.3",
		Info: &openapi3.Info{
			Title:       "osvBNG API",
			Description: "Northbound REST API for osvBNG - Open Source Virtual Broadband Network Gateway",
			Version:     version.Version,
		},
		Paths: &openapi3.Paths{},
	}

	addFixedEndpoints(spec, tagSet)
	if err := addShowEndpoints(spec, adapter, tagSet); err != nil {
		return nil, err
	}
	if err := addConfEndpoints(spec, adapter, tagSet); err != nil {
		return nil, err
	}
	if err := addOperEndpoints(spec, adapter, tagSet); err != nil {
		return nil, err
	}

	spec.Tags = openapi3.Tags{{Name: "General", Description: "General API endpoints"}}
	for _, category := range []string{"Show", "Config", "Oper"} {
		var subTags []string
		for tag := range tagSet {
			if strings.HasPrefix(tag, category+" / ") {
				subTags = append(subTags, tag)
			}
		}
		sort.Strings(subTags)
		for _, tag := range subTags {
			spec.Tags = append(spec.Tags, &openapi3.Tag{
				Name:        tag,
				Description: tag + " endpoints",
			})
		}
	}

	return spec, nil
}

func validateOpenAPIMetadata(adapter *Adapter) error {
	var problems []string

	for pattern, handler := range adapter.GetAllOperHandlers() {
		inputHandler, ok := handler.(oper.OperInputHandler)
		if !ok || inputHandler.InputType() == nil {
			problems = append(problems, fmt.Sprintf("oper handler %q missing InputType()", pattern))
		}
	}

	for pattern, handler := range adapter.GetAllConfHandlers() {
		if strings.HasPrefix(pattern, "_internal.") {
			continue
		}

		if typed, ok := handler.(conf.ValueTypeHandler); ok && typed.ValueType() != nil {
			continue
		}

		if _, ok := inferConfInputType(pattern); ok {
			continue
		}

		problems = append(problems, fmt.Sprintf("config handler %q requires ValueType()", pattern))
	}

	if len(problems) == 0 {
		return nil
	}

	sort.Strings(problems)
	return fmt.Errorf("northbound OpenAPI metadata validation failed:\n- %s", strings.Join(problems, "\n- "))
}

func addFixedEndpoints(spec *openapi3.T, tagSet map[string]bool) {
	tagSet["Show / Config"] = true
	tagSet["Config / Session"] = true

	spec.Paths.Set("/api", &openapi3.PathItem{
		Get: &openapi3.Operation{
			Tags:        []string{"General"},
			Summary:     "List all available API paths",
			OperationID: "listPaths",
			Responses: openapi3.NewResponses(
				openapi3.WithStatus(200, responseWithSchema("List of all registered handler paths", schemaFromType(reflect.TypeOf(pathsResponse{})))),
			),
		},
	})

	cfgSchema := schemaFromType(reflect.TypeOf(config.Config{}))
	configHistorySchema := schemaFromType(reflect.TypeOf(configHistoryResponse{}))
	configVersionSchema := schemaFromType(reflect.TypeOf(configVersionResponse{}))
	configSessionSchema := schemaFromType(reflect.TypeOf(configSessionCreateResponse{}))
	actionSchema := schemaFromType(reflect.TypeOf(actionResponse{}))
	diffSchema := schemaFromType(reflect.TypeOf(diffResponse{}))
	errorSchema := schemaFromType(reflect.TypeOf(errorResponse{}))

	spec.Paths.Set("/api/show/running-config", &openapi3.PathItem{
		Get: &openapi3.Operation{
			Tags:        []string{"Show / Config"},
			Summary:     "show running-config",
			OperationID: "show_running_config",
			Responses: openapi3.NewResponses(
				openapi3.WithStatus(200, responseWithSchema("Running configuration show result", showEnvelopeSchema(cfgSchema))),
				openapi3.WithStatus(500, responseWithSchema("Internal server error", errorSchema)),
			),
		},
	})

	spec.Paths.Set("/api/show/startup-config", &openapi3.PathItem{
		Get: &openapi3.Operation{
			Tags:        []string{"Show / Config"},
			Summary:     "show startup-config",
			OperationID: "show_startup_config",
			Responses: openapi3.NewResponses(
				openapi3.WithStatus(200, responseWithSchema("Startup configuration show result", showEnvelopeSchema(cfgSchema))),
				openapi3.WithStatus(500, responseWithSchema("Internal server error", errorSchema)),
			),
		},
	})

	spec.Paths.Set("/api/show/config/history", &openapi3.PathItem{
		Get: &openapi3.Operation{
			Tags:        []string{"Show / Config"},
			Summary:     "show config history",
			OperationID: "show_config_history",
			Responses: openapi3.NewResponses(
				openapi3.WithStatus(200, responseWithSchema("Configuration history show result", showEnvelopeSchema(configHistorySchema))),
				openapi3.WithStatus(500, responseWithSchema("Internal server error", errorSchema)),
			),
		},
	})

	spec.Paths.Set("/api/show/config/version/{version}", &openapi3.PathItem{
		Get: &openapi3.Operation{
			Tags:        []string{"Show / Config"},
			Summary:     "show config version",
			OperationID: "show_config_version",
			Parameters: openapi3.Parameters{
				pathParameter("version", &openapi3.SchemaRef{Value: &openapi3.Schema{Type: &openapi3.Types{"integer"}}}, "Configuration version number"),
			},
			Responses: openapi3.NewResponses(
				openapi3.WithStatus(200, responseWithSchema("Configuration version show result", showEnvelopeSchema(configVersionSchema))),
				openapi3.WithStatus(400, responseWithSchema("Invalid version", errorSchema)),
				openapi3.WithStatus(404, responseWithSchema("Configuration version not found", errorSchema)),
				openapi3.WithStatus(500, responseWithSchema("Internal server error", errorSchema)),
			),
		},
	})

	spec.Paths.Set("/api/running-config", &openapi3.PathItem{
		Get: &openapi3.Operation{
			Tags:        []string{"Show / Config"},
			Summary:     "Get the running configuration (deprecated alias)",
			OperationID: "getRunningConfig",
			Deprecated:  true,
			Responses: openapi3.NewResponses(
				openapi3.WithStatus(200, responseWithSchema("Current running configuration", cfgSchema)),
			),
		},
	})

	spec.Paths.Set("/api/startup-config", &openapi3.PathItem{
		Get: &openapi3.Operation{
			Tags:        []string{"Show / Config"},
			Summary:     "Get the startup configuration (deprecated alias)",
			OperationID: "getStartupConfig",
			Deprecated:  true,
			Responses: openapi3.NewResponses(
				openapi3.WithStatus(200, responseWithSchema("Startup configuration", cfgSchema)),
			),
		},
	})

	spec.Paths.Set("/api/config/session", &openapi3.PathItem{
		Post: &openapi3.Operation{
			Tags:        []string{"Config / Session"},
			Summary:     "Create a candidate configuration session",
			OperationID: "create_config_session",
			Responses: openapi3.NewResponses(
				openapi3.WithStatus(201, responseWithSchema("Candidate session created", configSessionSchema)),
				openapi3.WithStatus(409, responseWithSchema("Configuration lock conflict", errorSchema)),
				openapi3.WithStatus(500, responseWithSchema("Internal server error", errorSchema)),
			),
		},
	})

	spec.Paths.Set("/api/config/session/{session_id}/set/{path}", &openapi3.PathItem{
		Post: &openapi3.Operation{
			Tags:        []string{"Config / Session"},
			Summary:     "Apply a configuration change to a candidate session",
			Description: "Uses the same path normalization and body-flattening semantics as POST /api/set/... but applies changes to an existing candidate session.",
			OperationID: "set_config_session_path",
			Parameters: openapi3.Parameters{
				pathParameter("session_id", &openapi3.SchemaRef{Value: &openapi3.Schema{Type: &openapi3.Types{"string"}}}, "Candidate session identifier"),
				pathParameter("path", &openapi3.SchemaRef{Value: &openapi3.Schema{Type: &openapi3.Types{"string"}}}, "Slash-separated configuration path"),
			},
			RequestBody: &openapi3.RequestBodyRef{
				Value: &openapi3.RequestBody{
					Required: true,
					Content: openapi3.NewContentWithJSONSchemaRef(&openapi3.SchemaRef{
						Value: &openapi3.Schema{
							Description: "JSON value for the target configuration path. The effective schema matches the corresponding POST /api/set/... endpoint for the normalized path.",
						},
					}),
				},
			},
			Responses: openapi3.NewResponses(
				openapi3.WithStatus(200, responseWithSchema("Candidate session updated", actionSchema)),
				openapi3.WithStatus(400, responseWithSchema("Invalid config path or request body", errorSchema)),
				openapi3.WithStatus(404, responseWithSchema("Candidate session not found", errorSchema)),
				openapi3.WithStatus(500, responseWithSchema("Internal server error", errorSchema)),
			),
		},
	})

	spec.Paths.Set("/api/config/session/{session_id}/commit", &openapi3.PathItem{
		Post: &openapi3.Operation{
			Tags:        []string{"Config / Session"},
			Summary:     "Commit a candidate configuration session",
			Description: "Successful commit is terminal and closes the candidate session server-side.",
			OperationID: "commit_config_session",
			Parameters: openapi3.Parameters{
				pathParameter("session_id", &openapi3.SchemaRef{Value: &openapi3.Schema{Type: &openapi3.Types{"string"}}}, "Candidate session identifier"),
			},
			Responses: openapi3.NewResponses(
				openapi3.WithStatus(200, responseWithSchema("Candidate session committed", actionSchema)),
				openapi3.WithStatus(400, responseWithSchema("Commit rejected", errorSchema)),
				openapi3.WithStatus(404, responseWithSchema("Candidate session not found", errorSchema)),
				openapi3.WithStatus(500, responseWithSchema("Internal server error", errorSchema)),
			),
		},
	})

	spec.Paths.Set("/api/config/session/{session_id}/discard", &openapi3.PathItem{
		Post: &openapi3.Operation{
			Tags:        []string{"Config / Session"},
			Summary:     "Discard a candidate configuration session",
			Description: "Successful discard is terminal and closes the candidate session server-side.",
			OperationID: "discard_config_session",
			Parameters: openapi3.Parameters{
				pathParameter("session_id", &openapi3.SchemaRef{Value: &openapi3.Schema{Type: &openapi3.Types{"string"}}}, "Candidate session identifier"),
			},
			Responses: openapi3.NewResponses(
				openapi3.WithStatus(200, responseWithSchema("Candidate session discarded", actionSchema)),
				openapi3.WithStatus(404, responseWithSchema("Candidate session not found", errorSchema)),
				openapi3.WithStatus(500, responseWithSchema("Internal server error", errorSchema)),
			),
		},
	})

	spec.Paths.Set("/api/config/session/{session_id}/diff", &openapi3.PathItem{
		Get: &openapi3.Operation{
			Tags:        []string{"Config / Session"},
			Summary:     "Show pending changes for a candidate configuration session",
			OperationID: "diff_config_session",
			Parameters: openapi3.Parameters{
				pathParameter("session_id", &openapi3.SchemaRef{Value: &openapi3.Schema{Type: &openapi3.Types{"string"}}}, "Candidate session identifier"),
			},
			Responses: openapi3.NewResponses(
				openapi3.WithStatus(200, responseWithSchema("Candidate session diff", diffSchema)),
				openapi3.WithStatus(404, responseWithSchema("Candidate session not found", errorSchema)),
				openapi3.WithStatus(500, responseWithSchema("Internal server error", errorSchema)),
			),
		},
	})
}

func addShowEndpoints(spec *openapi3.T, adapter *Adapter, tagSet map[string]bool) error {
	handlers := adapter.GetAllShowHandlers()

	paths := sortedKeys(handlers)
	for _, pattern := range paths {
		handler := handlers[pattern]

		urlPath, params := dotPathToURLPath(pattern, "/api/show/")
		if typed, ok := handler.(show.ShowOptionsHandler); ok {
			optionParams, err := optionsTypeToParameters(typed.OptionsType())
			if err != nil {
				return fmt.Errorf("show handler %q options metadata: %w", pattern, err)
			}
			params = append(params, optionParams...)
		}

		tag := deriveTag("Show", pattern)
		tagSet[tag] = true

		responseSchema := &openapi3.SchemaRef{Value: &openapi3.Schema{Type: &openapi3.Types{"object"}}}
		if typed, ok := handler.(show.TypedShowHandler); ok && typed.OutputType() != nil {
			responseSchema = schemaFromType(reflect.TypeOf(typed.OutputType()))
		}

		showSchema := schemaFromType(reflect.TypeOf(showResponse{}))
		showSchema.Value.Properties["data"] = responseSchema

		op := &openapi3.Operation{
			Tags:        []string{tag},
			Summary:     handlerSummary("show", pattern, handler),
			Description: handlerDescription(handler),
			OperationID: operationID("show", pattern),
			Parameters:  params,
			Responses: openapi3.NewResponses(
				openapi3.WithStatus(200, responseWithSchema("Show command result", showSchema)),
				openapi3.WithStatus(500, responseWithSchema("Internal server error", schemaFromType(reflect.TypeOf(errorResponse{})))),
			),
		}

		item := spec.Paths.Value(urlPath)
		if item == nil {
			item = &openapi3.PathItem{}
		}
		item.Get = op
		spec.Paths.Set(urlPath, item)
	}

	return nil
}

func addConfEndpoints(spec *openapi3.T, adapter *Adapter, tagSet map[string]bool) error {
	handlers := adapter.GetAllConfHandlers()

	paths := sortedKeys(handlers)
	for _, pattern := range paths {
		if strings.HasPrefix(pattern, "_internal.") {
			continue
		}

		handler := handlers[pattern]
		requestSchema, err := confInputSchema(pattern, handler)
		if err != nil {
			return err
		}

		urlPath, params := dotPathToURLPath(pattern, "/api/set/")
		tag := deriveTag("Config", pattern)
		tagSet[tag] = true

		op := &openapi3.Operation{
			Tags:        []string{tag},
			Summary:     handlerSummary("set", pattern, handler),
			Description: handlerDescription(handler),
			OperationID: operationID("set", pattern),
			Parameters:  params,
			RequestBody: &openapi3.RequestBodyRef{
				Value: &openapi3.RequestBody{
					Required: true,
					Content:  openapi3.NewContentWithJSONSchemaRef(requestSchema),
				},
			},
			Responses: openapi3.NewResponses(
				openapi3.WithStatus(200, responseWithSchema("Configuration applied successfully", schemaFromType(reflect.TypeOf(statusResponse{})))),
				openapi3.WithStatus(409, responseWithSchema("Configuration lock conflict", schemaFromType(reflect.TypeOf(errorResponse{})))),
				openapi3.WithStatus(500, responseWithSchema("Internal server error", schemaFromType(reflect.TypeOf(errorResponse{})))),
			),
		}

		item := spec.Paths.Value(urlPath)
		if item == nil {
			item = &openapi3.PathItem{}
		}
		item.Post = op
		spec.Paths.Set(urlPath, item)
	}

	return nil
}

func addOperEndpoints(spec *openapi3.T, adapter *Adapter, tagSet map[string]bool) error {
	handlers := adapter.GetAllOperHandlers()

	paths := sortedKeys(handlers)
	for _, pattern := range paths {
		handler := handlers[pattern]
		inputHandler, ok := handler.(oper.OperInputHandler)
		if !ok || inputHandler.InputType() == nil {
			return fmt.Errorf("oper handler %q missing InputType()", pattern)
		}

		urlPath, params := dotPathToURLPath(pattern, "/api/exec/")
		if typed, ok := handler.(oper.OperOptionsHandler); ok {
			optionParams, err := optionsTypeToParameters(typed.OptionsType())
			if err != nil {
				return fmt.Errorf("oper handler %q options metadata: %w", pattern, err)
			}
			params = append(params, optionParams...)
		}

		tag := deriveTag("Oper", pattern)
		tagSet[tag] = true

		requestSchema := schemaFromType(reflect.TypeOf(inputHandler.InputType()))
		responseSchema := &openapi3.SchemaRef{Value: &openapi3.Schema{Type: &openapi3.Types{"object"}}}
		if typed, ok := handler.(oper.OperOutputHandler); ok && typed.OutputType() != nil {
			responseSchema = schemaFromType(reflect.TypeOf(typed.OutputType()))
		}

		op := &openapi3.Operation{
			Tags:        []string{tag},
			Summary:     handlerSummary("exec", pattern, handler),
			Description: handlerDescription(handler),
			OperationID: operationID("exec", pattern),
			Parameters:  params,
			RequestBody: &openapi3.RequestBodyRef{
				Value: &openapi3.RequestBody{
					Content: openapi3.NewContentWithJSONSchemaRef(requestSchema),
				},
			},
			Responses: openapi3.NewResponses(
				openapi3.WithStatus(200, responseWithSchema("Operational command result", responseSchema)),
				openapi3.WithStatus(500, responseWithSchema("Internal server error", schemaFromType(reflect.TypeOf(errorResponse{})))),
			),
		}

		item := spec.Paths.Value(urlPath)
		if item == nil {
			item = &openapi3.PathItem{}
		}
		item.Post = op
		spec.Paths.Set(urlPath, item)
	}

	return nil
}

func confInputSchema(pattern string, handler conf.Handler) (*openapi3.SchemaRef, error) {
	if typed, ok := handler.(conf.ValueTypeHandler); ok && typed.ValueType() != nil {
		return schemaFromType(reflect.TypeOf(typed.ValueType())), nil
	}

	t, ok := inferConfInputType(pattern)
	if !ok {
		return nil, fmt.Errorf("config handler %q requires ValueType()", pattern)
	}

	return schemaFromType(t), nil
}

func inferConfInputType(pattern string) (reflect.Type, bool) {
	parts := strings.Split(pattern, ".")
	t := reflect.TypeOf(config.Config{})

	for _, part := range parts {
		for t.Kind() == reflect.Ptr {
			t = t.Elem()
		}

		if strings.HasPrefix(part, "<") && strings.HasSuffix(part, ">") {
			switch t.Kind() {
			case reflect.Map, reflect.Slice:
				t = t.Elem()
				continue
			default:
				return nil, false
			}
		}

		if t.Kind() != reflect.Struct {
			return nil, false
		}

		matched := false
		for i := 0; i < t.NumField(); i++ {
			field := t.Field(i)
			name, _, ok := jsonFieldName(field)
			if !ok {
				continue
			}
			if name != part && name != strings.ReplaceAll(part, "_", "-") {
				continue
			}
			t = field.Type
			matched = true
			break
		}

		if !matched {
			return nil, false
		}
	}

	return t, true
}

func optionsTypeToParameters(optionsType interface{}) (openapi3.Parameters, error) {
	if optionsType == nil {
		return nil, fmt.Errorf("OptionsType() returned nil")
	}

	t := reflect.TypeOf(optionsType)
	for t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	if t.Kind() != reflect.Struct {
		return nil, fmt.Errorf("OptionsType() must be a struct or pointer to struct, got %s", t.Kind())
	}

	var params openapi3.Parameters
	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		if !field.IsExported() {
			continue
		}

		name := field.Tag.Get("query")
		if name == "" || name == "-" {
			continue
		}

		schema := schemaFromType(field.Type)
		applySchemaTags(schema.Value, field)

		param := &openapi3.Parameter{
			Name:        name,
			In:          "query",
			Required:    field.Tag.Get("required") == "true",
			Schema:      schema,
			Description: schema.Value.Description,
			Example:     schema.Value.Example,
		}
		params = append(params, &openapi3.ParameterRef{Value: param})
	}

	return params, nil
}

func dotPathToURLPath(pattern, prefix string) (string, openapi3.Parameters) {
	parts := strings.Split(pattern, ".")
	urlParts := make([]string, 0, len(parts))
	var params openapi3.Parameters

	for i, part := range parts {
		if strings.HasPrefix(part, "<") && strings.HasSuffix(part, ">") {
			inner := part[1 : len(part)-1]
			name := deriveParamName(inner, parts, i)
			urlParts = append(urlParts, "{"+name+"}")
			params = append(params, &openapi3.ParameterRef{
				Value: &openapi3.Parameter{
					Name:     name,
					In:       "path",
					Required: true,
					Schema:   wildcardTypeToSchema(inner),
				},
			})
			continue
		}

		urlParts = append(urlParts, part)
	}

	return prefix + strings.Join(urlParts, "/"), params
}

func deriveParamName(inner string, parts []string, index int) string {
	if strings.Contains(inner, ":") {
		typeParts := strings.SplitN(inner, ":", 2)
		typeName := typeParts[1]
		if index > 0 {
			preceding := parts[index-1]
			switch typeName {
			case "ip", "ipv4", "ipv6", "prefix", "mac", "protocol":
				return typeName
			default:
				return preceding + "_id"
			}
		}
		return typeName
	}

	if index > 0 {
		preceding := parts[index-1]
		if strings.HasSuffix(preceding, "s") && !strings.HasSuffix(preceding, "ss") {
			return preceding[:len(preceding)-1] + "_id"
		}
		return preceding + "_id"
	}

	return "id"
}

func wildcardTypeToSchema(inner string) *openapi3.SchemaRef {
	if strings.Contains(inner, ":") {
		typeParts := strings.SplitN(inner, ":", 2)
		switch typeParts[1] {
		case "ip", "ipv4", "ipv6":
			return &openapi3.SchemaRef{Value: &openapi3.Schema{Type: &openapi3.Types{"string"}, Format: "ip-address"}}
		case "prefix":
			return &openapi3.SchemaRef{Value: &openapi3.Schema{Type: &openapi3.Types{"string"}, Description: "IP prefix in CIDR notation"}}
		case "mac":
			return &openapi3.SchemaRef{Value: &openapi3.Schema{Type: &openapi3.Types{"string"}, Format: "mac-address"}}
		case "uint8", "uint16", "uint32", "uint64", "int8", "int16", "int32", "int64":
			return &openapi3.SchemaRef{Value: &openapi3.Schema{Type: &openapi3.Types{"integer"}}}
		}
	}

	return &openapi3.SchemaRef{Value: &openapi3.Schema{Type: &openapi3.Types{"string"}}}
}

func schemaFromType(t reflect.Type) *openapi3.SchemaRef {
	if t == nil {
		return &openapi3.SchemaRef{Value: &openapi3.Schema{Type: &openapi3.Types{"object"}}}
	}

	for t.Kind() == reflect.Ptr {
		t = t.Elem()
	}

	if t == reflect.TypeOf(time.Time{}) {
		return &openapi3.SchemaRef{Value: &openapi3.Schema{Type: &openapi3.Types{"string"}, Format: "date-time"}}
	}

	if t == reflect.TypeOf(time.Duration(0)) {
		return &openapi3.SchemaRef{Value: &openapi3.Schema{
			Type:        &openapi3.Types{"integer"},
			Description: "Duration in nanoseconds",
		}}
	}

	switch t.Kind() {
	case reflect.Bool:
		return &openapi3.SchemaRef{Value: &openapi3.Schema{Type: &openapi3.Types{"boolean"}}}
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return &openapi3.SchemaRef{Value: &openapi3.Schema{Type: &openapi3.Types{"integer"}}}
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return &openapi3.SchemaRef{Value: &openapi3.Schema{Type: &openapi3.Types{"integer"}}}
	case reflect.Float32, reflect.Float64:
		return &openapi3.SchemaRef{Value: &openapi3.Schema{Type: &openapi3.Types{"number"}}}
	case reflect.String:
		return &openapi3.SchemaRef{Value: &openapi3.Schema{Type: &openapi3.Types{"string"}}}
	case reflect.Slice:
		if t.Elem().Kind() == reflect.Uint8 {
			return &openapi3.SchemaRef{Value: &openapi3.Schema{Type: &openapi3.Types{"string"}, Format: "byte"}}
		}
		return &openapi3.SchemaRef{Value: &openapi3.Schema{
			Type:  &openapi3.Types{"array"},
			Items: schemaFromType(t.Elem()),
		}}
	case reflect.Map:
		return &openapi3.SchemaRef{Value: &openapi3.Schema{
			Type:                 &openapi3.Types{"object"},
			AdditionalProperties: openapi3.AdditionalProperties{Schema: schemaFromType(t.Elem())},
		}}
	case reflect.Struct:
		return structToSchema(t)
	default:
		return &openapi3.SchemaRef{Value: &openapi3.Schema{Type: &openapi3.Types{"object"}}}
	}
}

func structToSchema(t reflect.Type) *openapi3.SchemaRef {
	for t.Kind() == reflect.Ptr {
		t = t.Elem()
	}

	properties := openapi3.Schemas{}
	var required []string

	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		if !field.IsExported() {
			continue
		}

		name, omitempty, ok := jsonFieldName(field)
		if !ok {
			continue
		}

		prop := schemaFromType(field.Type)
		applySchemaTags(prop.Value, field)

		properties[name] = prop
		if !omitempty {
			required = append(required, name)
		}
	}

	schema := &openapi3.Schema{
		Type:       &openapi3.Types{"object"},
		Properties: properties,
	}
	if len(required) > 0 {
		schema.Required = required
	}

	return &openapi3.SchemaRef{Value: schema}
}

func jsonFieldName(field reflect.StructField) (name string, omitempty bool, ok bool) {
	tag := field.Tag.Get("json")
	if tag == "-" {
		return "", false, false
	}

	name = field.Name
	if tag != "" {
		parts := strings.Split(tag, ",")
		if parts[0] != "" {
			name = parts[0]
		}
		for _, opt := range parts[1:] {
			if opt == "omitempty" {
				omitempty = true
			}
		}
	}

	return name, omitempty, true
}

func applySchemaTags(schema *openapi3.Schema, field reflect.StructField) {
	if schema == nil {
		return
	}

	if desc := field.Tag.Get("description"); desc != "" {
		schema.Description = desc
	}
	if format := field.Tag.Get("format"); format != "" {
		schema.Format = format
	}
	if example := field.Tag.Get("example"); example != "" {
		schema.Example = parseTagValue(example, field.Type)
	}
	if def := field.Tag.Get("default"); def != "" {
		schema.Default = parseTagValue(def, field.Type)
	}
	if enum := field.Tag.Get("enum"); enum != "" {
		var values []interface{}
		for _, value := range strings.Split(enum, ",") {
			values = append(values, parseTagValue(strings.TrimSpace(value), field.Type))
		}
		schema.Enum = values
	}
}

func parseTagValue(value string, t reflect.Type) interface{} {
	for t.Kind() == reflect.Ptr {
		t = t.Elem()
	}

	switch t.Kind() {
	case reflect.Bool:
		if parsed, err := strconv.ParseBool(value); err == nil {
			return parsed
		}
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		if parsed, err := strconv.ParseInt(value, 10, 64); err == nil {
			return parsed
		}
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		if parsed, err := strconv.ParseUint(value, 10, 64); err == nil {
			return parsed
		}
	case reflect.Float32, reflect.Float64:
		if parsed, err := strconv.ParseFloat(value, 64); err == nil {
			return parsed
		}
	case reflect.Slice:
		if t.Elem().Kind() == reflect.String {
			var items []string
			for _, item := range strings.Split(value, ",") {
				items = append(items, strings.TrimSpace(item))
			}
			return items
		}
	}

	return value
}

func responseWithSchema(description string, schema *openapi3.SchemaRef) *openapi3.ResponseRef {
	return &openapi3.ResponseRef{
		Value: &openapi3.Response{
			Description: ptr(description),
			Content:     openapi3.NewContentWithJSONSchemaRef(schema),
		},
	}
}

func showEnvelopeSchema(dataSchema *openapi3.SchemaRef) *openapi3.SchemaRef {
	showSchema := schemaFromType(reflect.TypeOf(showResponse{}))
	showSchema.Value.Properties["data"] = dataSchema
	return showSchema
}

func pathParameter(name string, schema *openapi3.SchemaRef, description string) *openapi3.ParameterRef {
	return &openapi3.ParameterRef{
		Value: &openapi3.Parameter{
			Name:        name,
			In:          "path",
			Required:    true,
			Schema:      schema,
			Description: description,
		},
	}
}

func operationID(prefix, pattern string) string {
	value := prefix + "_" + strings.ReplaceAll(strings.ReplaceAll(pattern, ".", "_"), "<*>", "param")
	return wildcardRe.ReplaceAllString(value, "param")
}

func deriveTag(category, pattern string) string {
	parts := strings.Split(pattern, ".")
	if len(parts) == 0 {
		return category
	}

	group := parts[0]
	if len(group) > 0 {
		group = strings.ToUpper(group[:1]) + group[1:]
	}

	return category + " / " + group
}

func handlerSummary(prefix, pattern string, handler interface{}) string {
	switch h := handler.(type) {
	case show.ShowSummaryHandler:
		if summary := strings.TrimSpace(h.Summary()); summary != "" {
			return summary
		}
	case oper.OperSummaryHandler:
		if summary := strings.TrimSpace(h.Summary()); summary != "" {
			return summary
		}
	case conf.SummaryHandler:
		if summary := strings.TrimSpace(h.Summary()); summary != "" {
			return summary
		}
	}

	return prefix + " " + strings.ReplaceAll(pattern, ".", " ")
}

func handlerDescription(handler interface{}) string {
	switch h := handler.(type) {
	case show.ShowDescriptionHandler:
		return strings.TrimSpace(h.Description())
	case oper.OperDescriptionHandler:
		return strings.TrimSpace(h.Description())
	case conf.DescriptionHandler:
		return strings.TrimSpace(h.Description())
	default:
		return ""
	}
}

func sortedKeys[T any](m map[string]T) []string {
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func ptr(value string) *string {
	return &value
}
