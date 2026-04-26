// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package pagination

import (
	"fmt"
	"net/url"
	"reflect"
	"sort"
	"strconv"
)

const (
	DefaultLimit = 100
	MaxLimit     = 1000
)

type Request struct {
	Limit  int
	Offset int
}

type Meta struct {
	Limit    int  `json:"limit"`
	Offset   int  `json:"offset"`
	Returned int  `json:"returned"`
	Total    int  `json:"total"`
	HasMore  bool `json:"has_more"`
}

type Page struct {
	Items     interface{}
	Meta      Meta
	Paginated bool
}

func RequestFromQuery(q url.Values) Request {
	r := Request{Limit: DefaultLimit, Offset: 0}

	if raw := q.Get("limit"); raw != "" {
		if v, err := strconv.Atoi(raw); err == nil && v > 0 {
			r.Limit = v
		}
	}
	if r.Limit > MaxLimit {
		r.Limit = MaxLimit
	}

	if raw := q.Get("offset"); raw != "" {
		if v, err := strconv.Atoi(raw); err == nil && v >= 0 {
			r.Offset = v
		}
	}

	return r
}

func Paginate(data interface{}, req Request, sortKey string) (Page, error) {
	v := reflect.ValueOf(data)
	for v.Kind() == reflect.Ptr || v.Kind() == reflect.Interface {
		if v.IsNil() {
			return Page{Items: data, Paginated: false}, nil
		}
		v = v.Elem()
	}

	if v.Kind() != reflect.Slice && v.Kind() != reflect.Array {
		return Page{Items: data, Paginated: false}, nil
	}

	total := v.Len()

	if sortKey != "" && total > 1 {
		if err := sortSlice(v, sortKey); err != nil {
			return Page{}, err
		}
	}

	start := req.Offset
	if start > total {
		start = total
	}
	end := start + req.Limit
	if end > total {
		end = total
	}

	page := v.Slice(start, end).Interface()
	returned := end - start

	return Page{
		Items:     page,
		Paginated: true,
		Meta: Meta{
			Limit:    req.Limit,
			Offset:   req.Offset,
			Returned: returned,
			Total:    total,
			HasMore:  end < total,
		},
	}, nil
}

func sortSlice(v reflect.Value, sortKey string) error {
	if v.Len() == 0 {
		return nil
	}

	elemType := concreteElemType(v)
	if elemType == nil {
		return fmt.Errorf("pagination: sort_key %q: no non-nil element to derive type from", sortKey)
	}

	if elemType.Kind() != reflect.Struct {
		return fmt.Errorf("pagination: sort_key %q requires struct elements, got %s", sortKey, elemType.Kind())
	}

	field, ok := findField(elemType, sortKey)
	if !ok {
		return fmt.Errorf("pagination: sort_key %q not found on %s", sortKey, elemType.Name())
	}

	less, err := comparator(field.Type)
	if err != nil {
		return fmt.Errorf("pagination: sort_key %q: %w", sortKey, err)
	}

	sort.SliceStable(v.Interface(), func(i, j int) bool {
		ai := elemAt(v, i)
		aj := elemAt(v, j)
		if !ai.IsValid() {
			return false
		}
		if !aj.IsValid() {
			return true
		}
		fi, ok := lookupField(ai, sortKey)
		if !ok {
			return false
		}
		fj, ok := lookupField(aj, sortKey)
		if !ok {
			return true
		}
		return less(fi, fj)
	})

	return nil
}

func concreteElemType(v reflect.Value) reflect.Type {
	t := v.Type().Elem()
	for t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	if t.Kind() != reflect.Interface {
		return t
	}
	for i := 0; i < v.Len(); i++ {
		e := elemAt(v, i)
		if e.IsValid() {
			return e.Type()
		}
	}
	return nil
}

func lookupField(elem reflect.Value, jsonName string) (reflect.Value, bool) {
	dyn, ok := findField(elem.Type(), jsonName)
	if !ok {
		return reflect.Value{}, false
	}
	return elem.FieldByIndex(dyn.Index), true
}

func elemAt(v reflect.Value, i int) reflect.Value {
	e := v.Index(i)
	for e.Kind() == reflect.Ptr || e.Kind() == reflect.Interface {
		if e.IsNil() {
			return reflect.Value{}
		}
		e = e.Elem()
	}
	return e
}

func findField(t reflect.Type, jsonName string) (reflect.StructField, bool) {
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		if !f.IsExported() {
			continue
		}
		name, _, ok := jsonFieldName(f)
		if !ok {
			continue
		}
		if name == jsonName {
			return f, true
		}
	}
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		if !f.IsExported() || !f.Anonymous {
			continue
		}
		ft := f.Type
		for ft.Kind() == reflect.Ptr {
			ft = ft.Elem()
		}
		if ft.Kind() == reflect.Struct {
			if nested, ok := findField(ft, jsonName); ok {
				nested.Index = append([]int{i}, nested.Index...)
				return nested, true
			}
		}
	}
	return reflect.StructField{}, false
}

func jsonFieldName(f reflect.StructField) (string, bool, bool) {
	tag := f.Tag.Get("json")
	if tag == "-" {
		return "", false, false
	}
	name := f.Name
	omitempty := false
	if tag != "" {
		parts := splitTag(tag)
		if parts[0] != "" {
			name = parts[0]
		}
		for _, p := range parts[1:] {
			if p == "omitempty" {
				omitempty = true
			}
		}
	}
	return name, omitempty, true
}

func splitTag(tag string) []string {
	var out []string
	start := 0
	for i := 0; i < len(tag); i++ {
		if tag[i] == ',' {
			out = append(out, tag[start:i])
			start = i + 1
		}
	}
	out = append(out, tag[start:])
	return out
}

func comparator(t reflect.Type) (func(a, b reflect.Value) bool, error) {
	for t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	switch t.Kind() {
	case reflect.String:
		return func(a, b reflect.Value) bool { return derefValue(a).String() < derefValue(b).String() }, nil
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return func(a, b reflect.Value) bool { return derefValue(a).Int() < derefValue(b).Int() }, nil
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return func(a, b reflect.Value) bool { return derefValue(a).Uint() < derefValue(b).Uint() }, nil
	case reflect.Float32, reflect.Float64:
		return func(a, b reflect.Value) bool { return derefValue(a).Float() < derefValue(b).Float() }, nil
	case reflect.Slice:
		if t.Elem().Kind() == reflect.Uint8 {
			return func(a, b reflect.Value) bool {
				return string(derefValue(a).Bytes()) < string(derefValue(b).Bytes())
			}, nil
		}
	}
	return nil, fmt.Errorf("unsupported sort field kind %s", t.Kind())
}

func derefValue(v reflect.Value) reflect.Value {
	for v.Kind() == reflect.Ptr || v.Kind() == reflect.Interface {
		if v.IsNil() {
			return reflect.Value{}
		}
		v = v.Elem()
	}
	return v
}
