// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package orderedjson

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
)

// Object is a JSON object that remembers the order its keys arrived in.
// Decoding server output into Object instead of map[string]any keeps the
// field order the server marshalled (struct declaration order), so the CLI
// renders columns and tree fields in a stable, meaningful order rather than
// Go's randomized map iteration order.
type Object struct {
	Keys []string
	Vals map[string]any
}

// Get returns the value for key and whether it was present.
func (o *Object) Get(key string) (any, bool) {
	v, ok := o.Vals[key]
	return v, ok
}

// MarshalJSON re-emits the object with keys in their original order so the
// `| json` output is ordered too.
func (o *Object) MarshalJSON() ([]byte, error) {
	if o == nil {
		return []byte("null"), nil
	}
	var buf bytes.Buffer
	buf.WriteByte('{')
	for i, k := range o.Keys {
		if i > 0 {
			buf.WriteByte(',')
		}
		kb, err := json.Marshal(k)
		if err != nil {
			return nil, err
		}
		buf.Write(kb)
		buf.WriteByte(':')
		vb, err := json.Marshal(o.Vals[k])
		if err != nil {
			return nil, err
		}
		buf.Write(vb)
	}
	buf.WriteByte('}')
	return buf.Bytes(), nil
}

// Decode parses raw JSON, representing objects as *Object (order-preserving),
// arrays as []any, and numbers as json.Number so large integer counters keep
// their exact textual form. Empty input decodes to nil.
func Decode(raw []byte) (any, error) {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	v, err := decodeValue(dec)
	if err == io.EOF {
		return nil, nil
	}
	return v, err
}

func decodeValue(dec *json.Decoder) (any, error) {
	tok, err := dec.Token()
	if err != nil {
		return nil, err
	}
	if delim, ok := tok.(json.Delim); ok {
		switch delim {
		case '{':
			return decodeObject(dec)
		case '[':
			return decodeArray(dec)
		}
	}
	return tok, nil
}

func decodeObject(dec *json.Decoder) (*Object, error) {
	obj := &Object{Vals: make(map[string]any)}
	for dec.More() {
		keyTok, err := dec.Token()
		if err != nil {
			return nil, err
		}
		key, ok := keyTok.(string)
		if !ok {
			return nil, fmt.Errorf("object key is not a string: %v", keyTok)
		}
		val, err := decodeValue(dec)
		if err != nil {
			return nil, err
		}
		if _, exists := obj.Vals[key]; !exists {
			obj.Keys = append(obj.Keys, key)
		}
		obj.Vals[key] = val
	}
	if _, err := dec.Token(); err != nil { // closing '}'
		return nil, err
	}
	return obj, nil
}

func decodeArray(dec *json.Decoder) ([]any, error) {
	arr := []any{}
	for dec.More() {
		val, err := decodeValue(dec)
		if err != nil {
			return nil, err
		}
		arr = append(arr, val)
	}
	if _, err := dec.Token(); err != nil { // closing ']'
		return nil, err
	}
	return arr, nil
}
