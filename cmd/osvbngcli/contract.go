package main

import (
	"fmt"
	"net/http"
	"sort"
	"strings"

	"github.com/getkin/kin-openapi/openapi3"
)

type CommandKind string

const (
	CommandShow CommandKind = "show"
	CommandSet  CommandKind = "set"
	CommandExec CommandKind = "exec"
)

type BodyMode int

const (
	BodyModeNone BodyMode = iota
	BodyModeScalar
	BodyModeFlattened
	BodyModeRawJSON
)

type ValueKind int

const (
	ValueString ValueKind = iota
	ValueInteger
	ValueNumber
	ValueBoolean
)

type Contract struct {
	Commands []*GeneratedCommand
}

type GeneratedCommand struct {
	Kind        CommandKind
	Method      string
	Path        string
	Summary     string
	Description string
	Segments    []CommandSegment
	QueryFlags  []*FlagSpec
	Body        BodySpec
}

type CommandSegment struct {
	Literal string
	Param   *ParamSpec
}

type ParamSpec struct {
	Name        string
	Description string
	Kind        ValueKind
	Enum        []string
}

type FlagSpec struct {
	CLIName      string
	SourceName   string
	Description  string
	Example      interface{}
	Required     bool
	Repeated     bool
	Kind         ValueKind
	Enum         []string
	Location     FlagLocation
	BodyPath     []string
	TopLevelBody bool
}

type FlagLocation int

const (
	FlagQuery FlagLocation = iota
	FlagBody
)

type BodySpec struct {
	Mode  BodyMode
	Flags []*FlagSpec
}

func buildContract(spec *openapi3.T) (*Contract, error) {
	if spec == nil || spec.Paths == nil {
		return nil, fmt.Errorf("OpenAPI spec has no paths")
	}

	contract := &Contract{
		Commands: make([]*GeneratedCommand, 0),
	}

	for _, path := range spec.Paths.InMatchingOrder() {
		item := spec.Paths.Value(path)
		if item == nil {
			continue
		}

		for _, methodOp := range []struct {
			method string
			op     *openapi3.Operation
		}{
			{method: http.MethodGet, op: item.Get},
			{method: http.MethodPost, op: item.Post},
		} {
			candidate, err := buildCommand(methodOp.method, path, methodOp.op)
			if err != nil {
				return nil, err
			}
			if candidate != nil {
				contract.Commands = append(contract.Commands, candidate)
			}
		}
	}

	sort.Slice(contract.Commands, func(i, j int) bool {
		left := contract.Commands[i]
		right := contract.Commands[j]

		if len(left.Segments) != len(right.Segments) {
			return len(left.Segments) < len(right.Segments)
		}
		return left.Path < right.Path
	})

	return contract, nil
}

func buildCommand(method, path string, op *openapi3.Operation) (*GeneratedCommand, error) {
	if op == nil || op.Deprecated {
		return nil, nil
	}

	var (
		prefix string
		kind   CommandKind
	)

	switch {
	case method == http.MethodGet && strings.HasPrefix(path, "/api/show/"):
		prefix = "/api/show/"
		kind = CommandShow
	case method == http.MethodPost && strings.HasPrefix(path, "/api/set/"):
		prefix = "/api/set/"
		kind = CommandSet
	case method == http.MethodPost && strings.HasPrefix(path, "/api/exec/"):
		prefix = "/api/exec/"
		kind = CommandExec
	default:
		return nil, nil
	}

	trimmed := strings.Trim(strings.TrimPrefix(path, prefix), "/")
	if trimmed == "" {
		return nil, nil
	}

	segments, err := buildSegments(trimmed, op.Parameters)
	if err != nil {
		return nil, fmt.Errorf("%s %s segments: %w", method, path, err)
	}
	segments = append([]CommandSegment{{Literal: string(kind)}}, segments...)

	queryFlags, err := buildQueryFlags(op.Parameters)
	if err != nil {
		return nil, fmt.Errorf("%s %s query flags: %w", method, path, err)
	}

	body, err := buildBodySpec(op.RequestBody)
	if err != nil {
		return nil, fmt.Errorf("%s %s body: %w", method, path, err)
	}

	return &GeneratedCommand{
		Kind:        kind,
		Method:      method,
		Path:        path,
		Summary:     strings.TrimSpace(op.Summary),
		Description: strings.TrimSpace(op.Description),
		Segments:    segments,
		QueryFlags:  queryFlags,
		Body:        body,
	}, nil
}

func buildSegments(trimmedPath string, params openapi3.Parameters) ([]CommandSegment, error) {
	paramMap := map[string]*openapi3.Parameter{}
	for _, ref := range params {
		if ref == nil || ref.Value == nil || ref.Value.In != "path" {
			continue
		}
		paramMap[ref.Value.Name] = ref.Value
	}

	parts := strings.Split(trimmedPath, "/")
	segments := make([]CommandSegment, 0, len(parts))
	for _, part := range parts {
		if strings.HasPrefix(part, "{") && strings.HasSuffix(part, "}") {
			name := strings.TrimSuffix(strings.TrimPrefix(part, "{"), "}")
			param := paramMap[name]
			if param == nil {
				return nil, fmt.Errorf("missing path parameter metadata for %q", name)
			}
			segments = append(segments, CommandSegment{
				Param: buildParamSpec(param),
			})
			continue
		}

		segments = append(segments, CommandSegment{Literal: part})
	}

	return segments, nil
}

func buildParamSpec(param *openapi3.Parameter) *ParamSpec {
	kind, enum := scalarSchemaKind(param.Schema)
	return &ParamSpec{
		Name:        param.Name,
		Description: strings.TrimSpace(param.Description),
		Kind:        kind,
		Enum:        enum,
	}
}

func buildQueryFlags(params openapi3.Parameters) ([]*FlagSpec, error) {
	flags := make([]*FlagSpec, 0)
	for _, ref := range params {
		if ref == nil || ref.Value == nil || ref.Value.In != "query" {
			continue
		}

		flag, err := buildQueryFlag(ref.Value)
		if err != nil {
			return nil, err
		}
		flags = append(flags, flag)
	}

	sort.Slice(flags, func(i, j int) bool {
		return flags[i].CLIName < flags[j].CLIName
	})

	return flags, nil
}

func buildQueryFlag(param *openapi3.Parameter) (*FlagSpec, error) {
	if param.Schema == nil || param.Schema.Value == nil {
		return nil, fmt.Errorf("query parameter %q missing schema", param.Name)
	}

	schema := param.Schema.Value
	flag := &FlagSpec{
		CLIName:     cliFlagName(param.Name),
		SourceName:  param.Name,
		Description: firstNonEmpty(strings.TrimSpace(param.Description), strings.TrimSpace(schema.Description)),
		Example:     firstNonNil(param.Example, schema.Example),
		Required:    param.Required,
		Location:    FlagQuery,
	}

	if schema.Type != nil && schema.Type.Is("array") {
		if schema.Items == nil || schema.Items.Value == nil {
			return nil, fmt.Errorf("query parameter %q has unsupported array schema", param.Name)
		}
		kind, enum := scalarSchemaKind(schema.Items)
		flag.Kind = kind
		flag.Enum = enum
		flag.Repeated = true
		return flag, nil
	}

	kind, enum := scalarSchemaKind(param.Schema)
	flag.Kind = kind
	flag.Enum = enum
	return flag, nil
}

func buildBodySpec(bodyRef *openapi3.RequestBodyRef) (BodySpec, error) {
	if bodyRef == nil || bodyRef.Value == nil {
		return BodySpec{Mode: BodyModeNone}, nil
	}

	content := bodyRef.Value.Content.Get("application/json")
	if content == nil || content.Schema == nil || content.Schema.Value == nil {
		return BodySpec{Mode: BodyModeNone}, nil
	}

	schema := content.Schema.Value
	required := bodyRef.Value.Required

	switch {
	case schema.Type != nil && schema.Type.Is("object"):
		flags, flattenable, err := flattenObjectSchema(content.Schema, nil, required)
		if err != nil {
			return BodySpec{}, err
		}
		if !flattenable {
			return BodySpec{
				Mode: BodyModeRawJSON,
				Flags: []*FlagSpec{{
					CLIName:     "json",
					SourceName:  "json",
					Description: "Raw JSON request body",
					Required:    true,
					Location:    FlagBody,
				}},
			}, nil
		}

		sort.Slice(flags, func(i, j int) bool {
			return flags[i].CLIName < flags[j].CLIName
		})

		return BodySpec{Mode: BodyModeFlattened, Flags: flags}, nil
	case schema.Type != nil && schema.Type.Is("array"):
		if schema.Items == nil || schema.Items.Value == nil {
			return BodySpec{}, fmt.Errorf("top-level array request body missing item schema")
		}
		if schema.Items.Value.Type != nil && (schema.Items.Value.Type.Is("object") || schema.Items.Value.Type.Is("array")) {
			return BodySpec{
				Mode: BodyModeRawJSON,
				Flags: []*FlagSpec{{
					CLIName:     "json",
					SourceName:  "json",
					Description: "Raw JSON request body",
					Required:    true,
					Location:    FlagBody,
				}},
			}, nil
		}

		kind, enum := scalarSchemaKind(schema.Items)
		return BodySpec{
			Mode: BodyModeScalar,
			Flags: []*FlagSpec{{
				CLIName:      "value",
				SourceName:   "value",
				Description:  strings.TrimSpace(schema.Description),
				Required:     required,
				Repeated:     true,
				Kind:         kind,
				Enum:         enum,
				Location:     FlagBody,
				TopLevelBody: true,
			}},
		}, nil
	default:
		kind, enum := scalarSchemaKind(content.Schema)
		return BodySpec{
			Mode: BodyModeScalar,
			Flags: []*FlagSpec{{
				CLIName:      "value",
				SourceName:   "value",
				Description:  strings.TrimSpace(schema.Description),
				Required:     required,
				Kind:         kind,
				Enum:         enum,
				Location:     FlagBody,
				TopLevelBody: true,
			}},
		}, nil
	}
}

func flattenObjectSchema(schemaRef *openapi3.SchemaRef, path []string, requiredChain bool) ([]*FlagSpec, bool, error) {
	if schemaRef == nil || schemaRef.Value == nil {
		return nil, true, nil
	}

	schema := schemaRef.Value
	if hasAdditionalProperties(schema) {
		return nil, false, nil
	}
	if schema.Type == nil {
		return nil, false, nil
	}

	switch {
	case schema.Type.Is("object"):
		if len(schema.Properties) == 0 {
			return nil, true, nil
		}

		requiredSet := make(map[string]bool, len(schema.Required))
		for _, name := range schema.Required {
			requiredSet[name] = true
		}

		propertyNames := make([]string, 0, len(schema.Properties))
		for name := range schema.Properties {
			propertyNames = append(propertyNames, name)
		}
		sort.Strings(propertyNames)

		flags := make([]*FlagSpec, 0)
		for _, name := range propertyNames {
			childFlags, flattenable, err := flattenObjectSchema(schema.Properties[name], append(path, name), requiredChain && requiredSet[name])
			if err != nil {
				return nil, false, err
			}
			if !flattenable {
				return nil, false, nil
			}
			flags = append(flags, childFlags...)
		}
		return flags, true, nil
	case schema.Type.Is("array"):
		if schema.Items == nil || schema.Items.Value == nil {
			return nil, false, nil
		}
		if schema.Items.Value.Type != nil && (schema.Items.Value.Type.Is("object") || schema.Items.Value.Type.Is("array")) {
			return nil, false, nil
		}

		kind, enum := scalarSchemaKind(schema.Items)
		return []*FlagSpec{{
			CLIName:     cliFlagName(strings.Join(path, ".")),
			SourceName:  strings.Join(path, "."),
			Description: strings.TrimSpace(schema.Description),
			Example:     schema.Example,
			Required:    requiredChain,
			Repeated:    true,
			Kind:        kind,
			Enum:        enum,
			Location:    FlagBody,
			BodyPath:    append([]string(nil), path...),
		}}, true, nil
	default:
		kind, enum := scalarSchemaKind(schemaRef)
		return []*FlagSpec{{
			CLIName:     cliFlagName(strings.Join(path, ".")),
			SourceName:  strings.Join(path, "."),
			Description: strings.TrimSpace(schema.Description),
			Example:     schema.Example,
			Required:    requiredChain,
			Kind:        kind,
			Enum:        enum,
			Location:    FlagBody,
			BodyPath:    append([]string(nil), path...),
		}}, true, nil
	}
}

func hasAdditionalProperties(schema *openapi3.Schema) bool {
	if schema == nil {
		return false
	}

	return schema.AdditionalProperties.Has != nil || schema.AdditionalProperties.Schema != nil
}

func scalarSchemaKind(schemaRef *openapi3.SchemaRef) (ValueKind, []string) {
	if schemaRef == nil || schemaRef.Value == nil || schemaRef.Value.Type == nil {
		return ValueString, nil
	}

	schema := schemaRef.Value
	enum := make([]string, 0, len(schema.Enum))
	for _, value := range schema.Enum {
		enum = append(enum, fmt.Sprintf("%v", value))
	}

	switch {
	case schema.Type.Is("boolean"):
		if len(enum) == 0 {
			enum = []string{"true", "false"}
		}
		return ValueBoolean, enum
	case schema.Type.Is("integer"):
		return ValueInteger, enum
	case schema.Type.Is("number"):
		return ValueNumber, enum
	default:
		return ValueString, enum
	}
}

func cliFlagName(name string) string {
	parts := strings.Split(name, ".")
	for i, part := range parts {
		parts[i] = strings.ReplaceAll(part, "_", "-")
	}
	return strings.Join(parts, ".")
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func firstNonNil(values ...interface{}) interface{} {
	for _, value := range values {
		if value != nil {
			return value
		}
	}
	return nil
}
