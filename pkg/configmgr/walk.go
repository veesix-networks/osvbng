// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package configmgr

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/veesix-networks/osvbng/pkg/config"
	"github.com/veesix-networks/osvbng/pkg/handlers/conf"
)

func (cd *ConfigManager) emitStructFields(sess *session, basePath string, value interface{}, cfg *config.Config) {
	v := reflect.ValueOf(value)
	cd.walkValue(sess, basePath, v, cfg)
}

func (cd *ConfigManager) walkValue(sess *session, basePath string, v reflect.Value, cfg *config.Config) {
	if v.Kind() == reflect.Ptr {
		if v.IsNil() {
			return
		}
		v = v.Elem()
	}
	if v.Kind() != reflect.Struct {
		return
	}

	t := v.Type()
	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		fieldValue := v.Field(i)

		jsonTag := strings.Split(field.Tag.Get("json"), ",")[0]
		if jsonTag == "" || jsonTag == "-" {
			continue
		}

		if fieldValue.IsZero() {
			continue
		}

		var fieldPath string
		if basePath == "" {
			fieldPath = jsonTag
		} else {
			fieldPath = basePath + "." + jsonTag
		}

		if _, err := cd.registry.GetHandler(fieldPath); err == nil {
			if fieldValue.Kind() == reflect.Slice {
				for j := 0; j < fieldValue.Len(); j++ {
					sess.changes = append(sess.changes, &conf.HandlerContext{
						SessionID: sess.id,
						Path:      fieldPath,
						NewValue:  fieldValue.Index(j).Interface(),
						Config:    cfg,
					})
				}
			} else {
				sess.changes = append(sess.changes, &conf.HandlerContext{
					SessionID: sess.id,
					Path:      fieldPath,
					NewValue:  fieldValue.Interface(),
					Config:    cfg,
				})
			}
		}

		actual := fieldValue
		if actual.Kind() == reflect.Ptr {
			if actual.IsNil() {
				continue
			}
			actual = actual.Elem()
		}

		switch actual.Kind() {
		case reflect.Struct:
			cd.walkValue(sess, fieldPath, actual, cfg)
		case reflect.Map:
			cd.walkMap(sess, fieldPath, actual, cfg)
		case reflect.Slice:
			cd.walkSlice(sess, fieldPath, actual, cfg)
		}
	}
}

func (cd *ConfigManager) walkSlice(sess *session, basePath string, v reflect.Value, cfg *config.Config) {
	for i := 0; i < v.Len(); i++ {
		entryPath := fmt.Sprintf("%s.%d", basePath, i)
		elem := v.Index(i)

		if _, err := cd.registry.GetHandler(entryPath); err == nil {
			val := elem
			if val.Kind() == reflect.Struct && val.CanAddr() {
				val = val.Addr()
			}
			sess.changes = append(sess.changes, &conf.HandlerContext{
				SessionID: sess.id,
				Path:      entryPath,
				NewValue:  val.Interface(),
				Config:    cfg,
			})
			continue
		}

		actual := elem
		if actual.Kind() == reflect.Ptr {
			if actual.IsNil() {
				continue
			}
			actual = actual.Elem()
		}
		if actual.Kind() == reflect.Interface {
			actual = actual.Elem()
		}

		if actual.Kind() == reflect.Struct {
			cd.walkValue(sess, entryPath, actual, cfg)
		}
	}
}

func (cd *ConfigManager) walkMap(sess *session, basePath string, v reflect.Value, cfg *config.Config) {
	for _, key := range v.MapKeys() {
		entryPath := basePath + "." + key.String()
		entryValue := v.MapIndex(key)

		if _, err := cd.registry.GetHandler(entryPath); err == nil {
			sess.changes = append(sess.changes, &conf.HandlerContext{
				SessionID: sess.id,
				Path:      entryPath,
				NewValue:  entryValue.Interface(),
				Config:    cfg,
			})
		}

		actual := entryValue
		if actual.Kind() == reflect.Interface {
			actual = actual.Elem()
		}
		if actual.Kind() == reflect.Ptr {
			if actual.IsNil() {
				continue
			}
			actual = actual.Elem()
		}

		switch actual.Kind() {
		case reflect.Struct:
			cd.walkValue(sess, entryPath, actual, cfg)
		case reflect.Map:
			cd.walkMap(sess, entryPath, actual, cfg)
		}
	}
}
