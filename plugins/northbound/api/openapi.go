package api

import (
	"reflect"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/veesix-networks/osvbng/pkg/config"
	"github.com/veesix-networks/osvbng/pkg/handlers/oper"
	"github.com/veesix-networks/osvbng/pkg/handlers/show"
	"github.com/veesix-networks/osvbng/pkg/northbound"
)

var wildcardRe = regexp.MustCompile(`<([^>]+)>`)

func buildOpenAPISpec(adapter *northbound.Adapter) *openapi3.T {
	tagSet := map[string]bool{"General": true}

	spec := &openapi3.T{
		OpenAPI: "3.0.3",
		Info: &openapi3.Info{
			Title:       "osvBNG API",
			Description: "Northbound REST API for osvBNG - Open Source Virtual Broadband Network Gateway",
			Version:     "1.0.0",
		},
		Paths: &openapi3.Paths{},
	}

	addFixedEndpoints(spec)
	addShowEndpoints(spec, adapter, tagSet)
	addConfEndpoints(spec, adapter, tagSet)
	addOperEndpoints(spec, adapter, tagSet)

	spec.Tags = openapi3.Tags{{Name: "General", Description: "General API endpoints"}}
	categories := []string{"Show", "Config", "Oper"}
	for _, cat := range categories {
		var subTags []string
		for tag := range tagSet {
			if strings.HasPrefix(tag, cat+" / ") {
				subTags = append(subTags, tag)
			}
		}
		sort.Strings(subTags)
		for _, tag := range subTags {
			desc := tag + " endpoints"
			spec.Tags = append(spec.Tags, &openapi3.Tag{Name: tag, Description: desc})
		}
	}

	return spec
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

func addFixedEndpoints(spec *openapi3.T) {
	spec.Paths.Set("/api", &openapi3.PathItem{
		Get: &openapi3.Operation{
			Tags:        []string{"General"},
			Summary:     "List all available API paths",
			OperationID: "listPaths",
			Responses: openapi3.NewResponses(
				openapi3.WithStatus(200, &openapi3.ResponseRef{
					Value: &openapi3.Response{
						Description: ptr("List of all registered handler paths"),
						Content: openapi3.NewContentWithJSONSchemaRef(
							schemaFromType(reflect.TypeOf(PathsResponse{})),
						),
					},
				}),
			),
		},
	})

	spec.Paths.Set("/api/running-config", &openapi3.PathItem{
		Get: &openapi3.Operation{
			Tags:        []string{"General"},
			Summary:     "Get the running configuration",
			OperationID: "getRunningConfig",
			Responses: openapi3.NewResponses(
				openapi3.WithStatus(200, &openapi3.ResponseRef{
					Value: &openapi3.Response{
						Description: ptr("Current running configuration"),
						Content: openapi3.NewContentWithJSONSchemaRef(
							schemaFromType(reflect.TypeOf(config.Config{})),
						),
					},
				}),
			),
		},
	})

	spec.Paths.Set("/api/startup-config", &openapi3.PathItem{
		Get: &openapi3.Operation{
			Tags:        []string{"General"},
			Summary:     "Get the startup configuration",
			OperationID: "getStartupConfig",
			Responses: openapi3.NewResponses(
				openapi3.WithStatus(200, &openapi3.ResponseRef{
					Value: &openapi3.Response{
						Description: ptr("Startup configuration"),
						Content: openapi3.NewContentWithJSONSchemaRef(
							schemaFromType(reflect.TypeOf(config.Config{})),
						),
					},
				}),
			),
		},
	})
}

func addShowEndpoints(spec *openapi3.T, adapter *northbound.Adapter, tagSet map[string]bool) {
	handlers := adapter.GetAllShowHandlers()

	paths := make([]string, 0, len(handlers))
	for pattern := range handlers {
		paths = append(paths, pattern)
	}
	sort.Strings(paths)

	for _, pattern := range paths {
		handler := handlers[pattern]
		urlPath, params := dotPathToURLPath(pattern, "/api/show/")
		operationID := "show_" + strings.ReplaceAll(strings.ReplaceAll(pattern, ".", "_"), "<*>", "param")
		operationID = wildcardRe.ReplaceAllString(operationID, "param")

		tag := deriveTag("Show", pattern)
		tagSet[tag] = true

		var responseSchema *openapi3.SchemaRef
		if typed, ok := handler.(show.TypedShowHandler); ok {
			responseSchema = schemaFromType(reflect.TypeOf(typed.OutputType()))
		} else {
			responseSchema = &openapi3.SchemaRef{Value: &openapi3.Schema{Type: &openapi3.Types{"object"}}}
		}

		showRespSchema := &openapi3.SchemaRef{
			Value: &openapi3.Schema{
				Type: &openapi3.Types{"object"},
				Properties: openapi3.Schemas{
					"path": {Value: &openapi3.Schema{Type: &openapi3.Types{"string"}}},
					"data": responseSchema,
				},
			},
		}

		operation := &openapi3.Operation{
			Tags:        []string{tag},
			Summary:     "show " + strings.ReplaceAll(pattern, ".", " "),
			OperationID: operationID,
			Parameters:  params,
			Responses: openapi3.NewResponses(
				openapi3.WithStatus(200, &openapi3.ResponseRef{
					Value: &openapi3.Response{
						Description: ptr("Show command result"),
						Content:     openapi3.NewContentWithJSONSchemaRef(showRespSchema),
					},
				}),
				openapi3.WithStatus(500, &openapi3.ResponseRef{
					Value: &openapi3.Response{
						Description: ptr("Internal server error"),
						Content: openapi3.NewContentWithJSONSchemaRef(
							schemaFromType(reflect.TypeOf(ErrorResponse{})),
						),
					},
				}),
			),
		}

		existing := spec.Paths.Value(urlPath)
		if existing == nil {
			existing = &openapi3.PathItem{}
		}
		existing.Get = operation
		spec.Paths.Set(urlPath, existing)
	}
}

func addConfEndpoints(spec *openapi3.T, adapter *northbound.Adapter, tagSet map[string]bool) {
	handlers := adapter.GetAllConfHandlers()

	paths := make([]string, 0, len(handlers))
	for pattern := range handlers {
		if strings.HasPrefix(pattern, "_internal.") {
			continue
		}
		paths = append(paths, pattern)
	}
	sort.Strings(paths)

	for _, pattern := range paths {
		urlPath, params := dotPathToURLPath(pattern, "/api/set/")
		operationID := "set_" + strings.ReplaceAll(strings.ReplaceAll(pattern, ".", "_"), "<*>", "param")
		operationID = wildcardRe.ReplaceAllString(operationID, "param")

		tag := deriveTag("Config", pattern)
		tagSet[tag] = true

		requestSchema := inferConfInputSchema(pattern)

		operation := &openapi3.Operation{
			Tags:        []string{tag},
			Summary:     "set " + strings.ReplaceAll(pattern, ".", " "),
			OperationID: operationID,
			Parameters:  params,
			RequestBody: &openapi3.RequestBodyRef{
				Value: &openapi3.RequestBody{
					Required: true,
					Content:  openapi3.NewContentWithJSONSchemaRef(requestSchema),
				},
			},
			Responses: openapi3.NewResponses(
				openapi3.WithStatus(200, &openapi3.ResponseRef{
					Value: &openapi3.Response{
						Description: ptr("Configuration applied successfully"),
						Content: openapi3.NewContentWithJSONSchemaRef(&openapi3.SchemaRef{
							Value: &openapi3.Schema{
								Type: &openapi3.Types{"object"},
								Properties: openapi3.Schemas{
									"status": {Value: &openapi3.Schema{Type: &openapi3.Types{"string"}, Enum: []interface{}{"ok"}}},
								},
							},
						}),
					},
				}),
				openapi3.WithStatus(500, &openapi3.ResponseRef{
					Value: &openapi3.Response{
						Description: ptr("Internal server error"),
						Content: openapi3.NewContentWithJSONSchemaRef(
							schemaFromType(reflect.TypeOf(ErrorResponse{})),
						),
					},
				}),
			),
		}

		existing := spec.Paths.Value(urlPath)
		if existing == nil {
			existing = &openapi3.PathItem{}
		}
		existing.Post = operation
		spec.Paths.Set(urlPath, existing)
	}
}

func addOperEndpoints(spec *openapi3.T, adapter *northbound.Adapter, tagSet map[string]bool) {
	handlers := adapter.GetAllOperHandlers()

	paths := make([]string, 0, len(handlers))
	for pattern := range handlers {
		paths = append(paths, pattern)
	}
	sort.Strings(paths)

	for _, pattern := range paths {
		handler := handlers[pattern]
		urlPath, params := dotPathToURLPath(pattern, "/api/exec/")
		operationID := "exec_" + strings.ReplaceAll(strings.ReplaceAll(pattern, ".", "_"), "<*>", "param")
		operationID = wildcardRe.ReplaceAllString(operationID, "param")

		tag := deriveTag("Oper", pattern)
		tagSet[tag] = true

		var requestSchema, responseSchema *openapi3.SchemaRef
		if typed, ok := handler.(oper.TypedOperHandler); ok {
			requestSchema = schemaFromType(reflect.TypeOf(typed.InputType()))
			responseSchema = schemaFromType(reflect.TypeOf(typed.OutputType()))
		} else {
			requestSchema = &openapi3.SchemaRef{Value: &openapi3.Schema{Type: &openapi3.Types{"object"}}}
			responseSchema = &openapi3.SchemaRef{Value: &openapi3.Schema{Type: &openapi3.Types{"object"}}}
		}

		operation := &openapi3.Operation{
			Tags:        []string{tag},
			Summary:     "exec " + strings.ReplaceAll(pattern, ".", " "),
			OperationID: operationID,
			Parameters:  params,
			RequestBody: &openapi3.RequestBodyRef{
				Value: &openapi3.RequestBody{
					Content: openapi3.NewContentWithJSONSchemaRef(requestSchema),
				},
			},
			Responses: openapi3.NewResponses(
				openapi3.WithStatus(200, &openapi3.ResponseRef{
					Value: &openapi3.Response{
						Description: ptr("Operational command result"),
						Content:     openapi3.NewContentWithJSONSchemaRef(responseSchema),
					},
				}),
				openapi3.WithStatus(500, &openapi3.ResponseRef{
					Value: &openapi3.Response{
						Description: ptr("Internal server error"),
						Content: openapi3.NewContentWithJSONSchemaRef(
							schemaFromType(reflect.TypeOf(ErrorResponse{})),
						),
					},
				}),
			),
		}

		existing := spec.Paths.Value(urlPath)
		if existing == nil {
			existing = &openapi3.PathItem{}
		}
		existing.Post = operation
		spec.Paths.Set(urlPath, existing)
	}
}

func dotPathToURLPath(pattern, prefix string) (string, openapi3.Parameters) {
	parts := strings.Split(pattern, ".")
	urlParts := make([]string, 0, len(parts))
	var params openapi3.Parameters

	for i, part := range parts {
		if strings.HasPrefix(part, "<") && strings.HasSuffix(part, ">") {
			inner := part[1 : len(part)-1] // e.g. "*:ip" or "*"
			paramName := deriveParamName(inner, parts, i)

			urlParts = append(urlParts, "{"+paramName+"}")
			params = append(params, &openapi3.ParameterRef{
				Value: &openapi3.Parameter{
					Name:     paramName,
					In:       "path",
					Required: true,
					Schema:   wildcardTypeToSchema(inner),
				},
			})
		} else {
			urlParts = append(urlParts, part)
		}
	}

	return prefix + strings.Join(urlParts, "/"), params
}

func deriveParamName(wildcardInner string, parts []string, index int) string {
	if strings.Contains(wildcardInner, ":") {
		typeParts := strings.SplitN(wildcardInner, ":", 2)
		typeName := typeParts[1]
		if index > 0 {
			preceding := parts[index-1]
			if typeName == "ip" || typeName == "ipv4" || typeName == "ipv6" || typeName == "prefix" || typeName == "mac" || typeName == "protocol" {
				return typeName
			}
			return preceding + "_id"
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

func wildcardTypeToSchema(wildcardInner string) *openapi3.SchemaRef {
	if strings.Contains(wildcardInner, ":") {
		typeParts := strings.SplitN(wildcardInner, ":", 2)
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

func inferConfInputSchema(pattern string) *openapi3.SchemaRef {
	parts := strings.Split(pattern, ".")
	t := reflect.TypeOf(config.Config{})

	for _, part := range parts {
		if strings.HasPrefix(part, "<") && strings.HasSuffix(part, ">") {
			if t.Kind() == reflect.Map {
				t = t.Elem()
				if t.Kind() == reflect.Ptr {
					t = t.Elem()
				}
				continue
			}
			if t.Kind() == reflect.Slice {
				t = t.Elem()
				if t.Kind() == reflect.Ptr {
					t = t.Elem()
				}
				continue
			}
			return &openapi3.SchemaRef{Value: &openapi3.Schema{Type: &openapi3.Types{"object"}}}
		}

		if t.Kind() == reflect.Ptr {
			t = t.Elem()
		}

		if t.Kind() != reflect.Struct {
			return &openapi3.SchemaRef{Value: &openapi3.Schema{Type: &openapi3.Types{"object"}}}
		}

		found := false
		for i := 0; i < t.NumField(); i++ {
			field := t.Field(i)
			jsonTag := field.Tag.Get("json")
			if jsonTag == "" || jsonTag == "-" {
				continue
			}
			tagName := strings.Split(jsonTag, ",")[0]
			if tagName == part || tagName == strings.ReplaceAll(part, "_", "-") {
				ft := field.Type
				if ft.Kind() == reflect.Ptr {
					ft = ft.Elem()
				}
				t = ft
				found = true
				break
			}
		}

		if !found {
			return &openapi3.SchemaRef{Value: &openapi3.Schema{Type: &openapi3.Types{"object"}}}
		}
	}

	return schemaFromType(reflect.TypeOf(reflect.New(t).Elem().Interface()))
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
		return &openapi3.SchemaRef{Value: &openapi3.Schema{Type: &openapi3.Types{"integer"}, Description: "Duration in nanoseconds"}}
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
		return &openapi3.SchemaRef{
			Value: &openapi3.Schema{
				Type:  &openapi3.Types{"array"},
				Items: schemaFromType(t.Elem()),
			},
		}

	case reflect.Map:
		return &openapi3.SchemaRef{
			Value: &openapi3.Schema{
				Type:                 &openapi3.Types{"object"},
				AdditionalProperties: openapi3.AdditionalProperties{Schema: schemaFromType(t.Elem())},
			},
		}

	case reflect.Struct:
		return structToSchema(t)

	case reflect.Interface:
		return &openapi3.SchemaRef{Value: &openapi3.Schema{Type: &openapi3.Types{"object"}}}
	}

	return &openapi3.SchemaRef{Value: &openapi3.Schema{Type: &openapi3.Types{"object"}}}
}

func structToSchema(t reflect.Type) *openapi3.SchemaRef {
	for t.Kind() == reflect.Ptr {
		t = t.Elem()
	}

	properties := openapi3.Schemas{}

	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		if !field.IsExported() {
			continue
		}

		jsonTag := field.Tag.Get("json")
		if jsonTag == "-" {
			continue
		}

		name := field.Name
		omitempty := false
		if jsonTag != "" {
			parts := strings.Split(jsonTag, ",")
			if parts[0] != "" {
				name = parts[0]
			}
			for _, opt := range parts[1:] {
				if opt == "omitempty" {
					omitempty = true
				}
			}
		}

		propSchema := schemaFromType(field.Type)

		if desc := field.Tag.Get("description"); desc != "" {
			propSchema.Value.Description = desc
		}

		_ = omitempty
		properties[name] = propSchema
	}

	return &openapi3.SchemaRef{
		Value: &openapi3.Schema{
			Type:       &openapi3.Types{"object"},
			Properties: properties,
		},
	}
}

func ptr(s string) *string {
	return &s
}
