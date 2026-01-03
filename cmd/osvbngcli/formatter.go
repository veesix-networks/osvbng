package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
)

type OutputFormat string

const (
	FormatCLI  OutputFormat = "cli"
	FormatJSON OutputFormat = "json"
	FormatYAML OutputFormat = "yaml"
)

type GenericFormatter struct{}

func NewGenericFormatter() *GenericFormatter {
	return &GenericFormatter{}
}

func (f *GenericFormatter) Format(data interface{}, format OutputFormat) (string, error) {
	switch format {
	case FormatJSON:
		return f.formatJSON(data)
	case FormatYAML:
		return f.formatYAML(data, 0)
	case FormatCLI:
		return f.formatCLI(data)
	default:
		return "", fmt.Errorf("unsupported format: %s", format)
	}
}

func (f *GenericFormatter) formatJSON(data interface{}) (string, error) {
	var buf bytes.Buffer
	encoder := json.NewEncoder(&buf)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(data); err != nil {
		return "", err
	}
	return buf.String(), nil
}

func (f *GenericFormatter) formatYAML(data interface{}, indent int) (string, error) {
	var sb strings.Builder
	prefix := strings.Repeat("  ", indent)

	v := reflect.ValueOf(data)

	for v.Kind() == reflect.Ptr {
		if v.IsNil() {
			return "", nil
		}
		v = v.Elem()
	}

	if v.Kind() == reflect.Interface {
		v = v.Elem()
	}

	switch v.Kind() {
	case reflect.Struct:
		t := v.Type()
		for i := 0; i < v.NumField(); i++ {
			field := t.Field(i)
			fieldValue := v.Field(i)

			if !field.IsExported() {
				continue
			}

			fieldName := field.Name
			if jsonTag := field.Tag.Get("json"); jsonTag != "" && jsonTag != "-" {
				parts := strings.Split(jsonTag, ",")
				if parts[0] != "" {
					fieldName = parts[0]
				}
			}

			fv := fieldValue
			if fv.Kind() == reflect.Interface {
				fv = fv.Elem()
			}

			if fv.Kind() == reflect.Struct || fv.Kind() == reflect.Slice || fv.Kind() == reflect.Array || fv.Kind() == reflect.Map {
				sb.WriteString(fmt.Sprintf("%s%s:\n", prefix, fieldName))
				nested, _ := f.formatYAML(fieldValue.Interface(), indent+1)
				sb.WriteString(nested)
			} else {
				sb.WriteString(fmt.Sprintf("%s%s: %v\n", prefix, fieldName, fieldValue.Interface()))
			}
		}

	case reflect.Slice, reflect.Array:
		for i := 0; i < v.Len(); i++ {
			elem := v.Index(i)
			if elem.Kind() == reflect.Interface {
				elem = elem.Elem()
			}

			if elem.Kind() == reflect.Struct || elem.Kind() == reflect.Map {
				sb.WriteString(fmt.Sprintf("%s-\n", prefix))
				nested, _ := f.formatYAML(v.Index(i).Interface(), indent+1)
				sb.WriteString(nested)
			} else {
				sb.WriteString(fmt.Sprintf("%s- %v\n", prefix, v.Index(i).Interface()))
			}
		}

	case reflect.Map:
		keys := v.MapKeys()
		keyStrs := make([]string, len(keys))
		for i, k := range keys {
			keyStrs[i] = fmt.Sprintf("%v", k.Interface())
		}

		for _, keyStr := range keyStrs {
			val := v.MapIndex(reflect.ValueOf(keyStr))
			if val.Kind() == reflect.Interface {
				val = val.Elem()
			}

			if val.Kind() == reflect.Struct || val.Kind() == reflect.Slice || val.Kind() == reflect.Array || val.Kind() == reflect.Map {
				sb.WriteString(fmt.Sprintf("%s%s:\n", prefix, keyStr))
				nested, _ := f.formatYAML(val.Interface(), indent+1)
				sb.WriteString(nested)
			} else {
				sb.WriteString(fmt.Sprintf("%s%s: %v\n", prefix, keyStr, val.Interface()))
			}
		}

	default:
		sb.WriteString(fmt.Sprintf("%s%v\n", prefix, v.Interface()))
	}

	return sb.String(), nil
}

func (f *GenericFormatter) formatCLI(data interface{}) (string, error) {
	if data == nil {
		return "No data\n", nil
	}

	v := reflect.ValueOf(data)

	if !v.IsValid() || (v.Kind() == reflect.Ptr && v.IsNil()) {
		return "No data\n", nil
	}

	for v.Kind() == reflect.Ptr {
		v = v.Elem()
	}

	switch v.Kind() {
	case reflect.Slice, reflect.Array:
		return f.formatTable(v)
	default:
		return f.formatTree(data, 0)
	}
}

func (f *GenericFormatter) formatTable(v reflect.Value) (string, error) {
	if v.Len() == 0 {
		return "No data\n", nil
	}

	first := v.Index(0)
	for first.Kind() == reflect.Ptr {
		first = first.Elem()
	}

	if first.Kind() == reflect.Interface {
		first = first.Elem()
	}

	if first.Kind() == reflect.Map {
		return f.formatMapSlice(v)
	}

	if first.Kind() != reflect.Struct {
		var sb strings.Builder
		for i := 0; i < v.Len(); i++ {
			sb.WriteString(fmt.Sprintf("%v\n", v.Index(i).Interface()))
		}
		return sb.String(), nil
	}

	t := first.Type()
	var headers []string
	var fields []reflect.StructField

	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		if !field.IsExported() {
			continue
		}

		fieldName := field.Name
		if jsonTag := field.Tag.Get("json"); jsonTag != "" && jsonTag != "-" {
			parts := strings.Split(jsonTag, ",")
			if parts[0] != "" {
				fieldName = parts[0]
			}
		}

		headers = append(headers, fieldName)
		fields = append(fields, field)
	}

	colWidths := make([]int, len(headers))
	for i, h := range headers {
		colWidths[i] = len(h)
	}

	for i := 0; i < v.Len(); i++ {
		elem := v.Index(i)
		for elem.Kind() == reflect.Ptr {
			elem = elem.Elem()
		}

		for j, field := range fields {
			fieldValue := elem.FieldByName(field.Name)
			valStr := fmt.Sprintf("%v", fieldValue.Interface())
			if len(valStr) > colWidths[j] {
				colWidths[j] = len(valStr)
			}
		}
	}

	var sb strings.Builder

	for i, h := range headers {
		sb.WriteString(fmt.Sprintf("%-*s  ", colWidths[i], h))
	}
	sb.WriteString("\n")

	for _, w := range colWidths {
		sb.WriteString(strings.Repeat("-", w) + "  ")
	}
	sb.WriteString("\n")

	for i := 0; i < v.Len(); i++ {
		elem := v.Index(i)
		for elem.Kind() == reflect.Ptr {
			elem = elem.Elem()
		}

		for j, field := range fields {
			fieldValue := elem.FieldByName(field.Name)
			sb.WriteString(fmt.Sprintf("%-*v  ", colWidths[j], fieldValue.Interface()))
		}
		sb.WriteString("\n")
	}

	return sb.String(), nil
}

func (f *GenericFormatter) formatMapSlice(v reflect.Value) (string, error) {
	if v.Len() == 0 {
		return "No data\n", nil
	}

	keysMap := make(map[string]bool)
	var keys []string

	for i := 0; i < v.Len(); i++ {
		elem := v.Index(i)
		if elem.Kind() == reflect.Interface {
			elem = elem.Elem()
		}

		if elem.Kind() != reflect.Map {
			continue
		}

		for _, key := range elem.MapKeys() {
			keyStr := fmt.Sprintf("%v", key.Interface())
			if !keysMap[keyStr] {
				keysMap[keyStr] = true
				keys = append(keys, keyStr)
			}
		}
	}

	if len(keys) == 0 {
		return "No data\n", nil
	}

	colWidths := make([]int, len(keys))
	for i, k := range keys {
		colWidths[i] = len(k)
	}

	for i := 0; i < v.Len(); i++ {
		elem := v.Index(i)
		if elem.Kind() == reflect.Interface {
			elem = elem.Elem()
		}

		for j, key := range keys {
			val := elem.MapIndex(reflect.ValueOf(key))
			if val.IsValid() {
				valStr := fmt.Sprintf("%v", val.Interface())
				if len(valStr) > colWidths[j] {
					colWidths[j] = len(valStr)
				}
			}
		}
	}

	var sb strings.Builder

	for i, k := range keys {
		sb.WriteString(fmt.Sprintf("%-*s  ", colWidths[i], k))
	}
	sb.WriteString("\n")

	for _, w := range colWidths {
		sb.WriteString(strings.Repeat("-", w) + "  ")
	}
	sb.WriteString("\n")

	for i := 0; i < v.Len(); i++ {
		elem := v.Index(i)
		if elem.Kind() == reflect.Interface {
			elem = elem.Elem()
		}

		for j, key := range keys {
			val := elem.MapIndex(reflect.ValueOf(key))
			valStr := "-"
			if val.IsValid() {
				valStr = fmt.Sprintf("%v", val.Interface())
			}
			sb.WriteString(fmt.Sprintf("%-*s  ", colWidths[j], valStr))
		}
		sb.WriteString("\n")
	}

	return sb.String(), nil
}

func (f *GenericFormatter) formatTree(data interface{}, indent int) (string, error) {
	var sb strings.Builder
	prefix := strings.Repeat("  ", indent)

	v := reflect.ValueOf(data)
	for v.Kind() == reflect.Ptr {
		if v.IsNil() {
			return "", nil
		}
		v = v.Elem()
	}

	switch v.Kind() {
	case reflect.Struct:
		t := v.Type()
		for i := 0; i < v.NumField(); i++ {
			field := t.Field(i)
			if !field.IsExported() {
				continue
			}

			fieldValue := v.Field(i)
			fieldName := field.Name
			if jsonTag := field.Tag.Get("json"); jsonTag != "" && jsonTag != "-" {
				parts := strings.Split(jsonTag, ",")
				if parts[0] != "" {
					fieldName = parts[0]
				}
			}

			if fieldValue.Kind() == reflect.Struct ||
				(fieldValue.Kind() == reflect.Ptr && !fieldValue.IsNil() && fieldValue.Elem().Kind() == reflect.Struct) ||
				fieldValue.Kind() == reflect.Slice || fieldValue.Kind() == reflect.Map {
				sb.WriteString(fmt.Sprintf("%s%s:\n", prefix, fieldName))
				nested, _ := f.formatTree(fieldValue.Interface(), indent+1)
				sb.WriteString(nested)
			} else {
				sb.WriteString(fmt.Sprintf("%s%s: %v\n", prefix, fieldName, fieldValue.Interface()))
			}
		}

	case reflect.Slice, reflect.Array:
		for i := 0; i < v.Len(); i++ {
			sb.WriteString(fmt.Sprintf("%s[%d]:\n", prefix, i))
			nested, _ := f.formatTree(v.Index(i).Interface(), indent+1)
			sb.WriteString(nested)
		}

	case reflect.Map:
		for _, key := range v.MapKeys() {
			val := v.MapIndex(key)

			needsRecursion := false
			if val.IsValid() {
				valKind := val.Kind()
				if valKind == reflect.Interface && !val.IsNil() {
					valKind = val.Elem().Kind()
				}
				needsRecursion = valKind == reflect.Struct || valKind == reflect.Map ||
					valKind == reflect.Slice || valKind == reflect.Array
			}

			if needsRecursion {
				sb.WriteString(fmt.Sprintf("%s%v:\n", prefix, key.Interface()))
				nested, _ := f.formatTree(v.MapIndex(key).Interface(), indent+1)
				sb.WriteString(nested)
			} else {
				sb.WriteString(fmt.Sprintf("%s%v: %v\n", prefix, key.Interface(), val.Interface()))
			}
		}

	default:
		sb.WriteString(fmt.Sprintf("%s%v\n", prefix, v.Interface()))
	}

	return sb.String(), nil
}
