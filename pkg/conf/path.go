package conf

import (
	"fmt"
	"reflect"
	"strconv"
	"strings"

	"github.com/veesix-networks/osvbng/pkg/conf/types"
)

func findFieldByPath(v reflect.Value, pathPart string) reflect.Value {
	t := v.Type()

	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		jsonTag := field.Tag.Get("json")
		if jsonTag != "" {
			tagName := strings.Split(jsonTag, ",")[0]
			if tagName == pathPart {
				return v.Field(i)
			}
		}
	}

	return v.FieldByNameFunc(func(name string) bool {
		return strings.EqualFold(name, pathPart)
	})
}

func getValueFromConfig(config *types.Config, path string) (interface{}, error) {
	parts := strings.Split(path, ".")
	if len(parts) == 0 {
		return nil, fmt.Errorf("empty path")
	}

	if parts[0] == "plugins" && len(parts) >= 2 && config.Plugins != nil {
		namespace := ""
		fieldStartIdx := -1

		for ns := range config.Plugins {
			nsParts := strings.Split(ns, ".")
			if len(parts) >= len(nsParts)+1 {
				matches := true
				for i, nsPart := range nsParts {
					if parts[i+1] != nsPart {
						matches = false
						break
					}
				}
				if matches {
					namespace = ns
					fieldStartIdx = len(nsParts) + 1
					break
				}
			}
		}

		if namespace == "" {
			return nil, nil
		}

		pluginCfg, ok := config.Plugins[namespace]
		if !ok {
			return nil, nil
		}

		if fieldStartIdx >= len(parts) {
			return pluginCfg, nil
		}

		current := pluginCfg
		for _, part := range parts[fieldStartIdx:] {
			v := reflect.ValueOf(current)

			if v.Kind() == reflect.Ptr {
				if v.IsNil() {
					return nil, nil
				}
				v = v.Elem()
			}

			switch v.Kind() {
			case reflect.Struct:
				field := findFieldByPath(v, part)
				if !field.IsValid() {
					return nil, fmt.Errorf("field not found: %s", part)
				}
				current = field.Interface()

			case reflect.Map:
				key := reflect.ValueOf(part)
				value := v.MapIndex(key)
				if !value.IsValid() {
					return nil, nil
				}
				current = value.Interface()

			default:
				return nil, fmt.Errorf("cannot navigate into type %s at %s", v.Kind(), part)
			}
		}

		return current, nil
	}

	var current interface{} = config

	for _, part := range parts {
		v := reflect.ValueOf(current)

		if v.Kind() == reflect.Ptr {
			if v.IsNil() {
				return nil, nil
			}
			v = v.Elem()
		}

		switch v.Kind() {
		case reflect.Struct:
			field := findFieldByPath(v, part)
			if !field.IsValid() {
				return nil, fmt.Errorf("field not found: %s", part)
			}
			current = field.Interface()

		case reflect.Map:
			key := reflect.ValueOf(part)
			value := v.MapIndex(key)
			if !value.IsValid() {
				return nil, nil
			}
			current = value.Interface()

		default:
			return nil, fmt.Errorf("cannot navigate into type %s at %s", v.Kind(), part)
		}
	}

	return current, nil
}

func setValueInConfig(config *types.Config, path string, value interface{}) error {
	parts := strings.Split(path, ".")
	if len(parts) == 0 {
		return fmt.Errorf("empty path")
	}

	if parts[0] == "plugins" && len(parts) >= 3 {
		if config.Plugins == nil {
			config.Plugins = make(map[string]interface{})
		}

		namespace := ""
		fieldStartIdx := -1

		for ns := range config.Plugins {
			nsParts := strings.Split(ns, ".")
			if len(parts) >= len(nsParts)+2 {
				matches := true
				for i, nsPart := range nsParts {
					if parts[i+1] != nsPart {
						matches = false
						break
					}
				}
				if matches {
					namespace = ns
					fieldStartIdx = len(nsParts) + 1
					break
				}
			}
		}

		if namespace == "" {
			return fmt.Errorf("plugin config not found in path: %s", path)
		}

		pluginCfg, ok := config.Plugins[namespace]
		if !ok {
			return fmt.Errorf("plugin config not found: %s", namespace)
		}

		current := reflect.ValueOf(pluginCfg)
		if current.Kind() == reflect.Ptr {
			if current.IsNil() {
				return fmt.Errorf("plugin config is nil: %s", namespace)
			}
			current = current.Elem()
		}

		for i := fieldStartIdx; i < len(parts)-1; i++ {
			part := parts[i]

			switch current.Kind() {
			case reflect.Struct:
				field := findFieldByPath(current, part)
				if !field.IsValid() {
					return fmt.Errorf("field not found: %s", part)
				}
				if !field.CanSet() {
					return fmt.Errorf("field not settable: %s", part)
				}
				current = field

			case reflect.Map:
				if current.IsNil() {
					current.Set(reflect.MakeMap(current.Type()))
				}

				key := reflect.ValueOf(part)
				mapValue := current.MapIndex(key)

				if !mapValue.IsValid() {
					elemType := current.Type().Elem()
					if elemType.Kind() == reflect.Ptr {
						newElem := reflect.New(elemType.Elem())
						current.SetMapIndex(key, newElem)
						current = newElem
					} else {
						newElem := reflect.New(elemType)
						current.SetMapIndex(key, newElem.Elem())
						current = newElem
					}
				} else {
					if mapValue.Kind() == reflect.Ptr {
						current = mapValue
					} else {
						newElem := reflect.New(mapValue.Type())
						newElem.Elem().Set(mapValue)
						current = newElem
					}
				}

			default:
				return fmt.Errorf("cannot navigate into type %s at %s", current.Kind(), part)
			}
		}

		lastPart := parts[len(parts)-1]

		if current.Kind() == reflect.Struct {
			field := findFieldByPath(current, lastPart)
			if !field.IsValid() {
				return fmt.Errorf("field not found: %s", lastPart)
			}
			if !field.CanSet() {
				return fmt.Errorf("field not settable: %s", lastPart)
			}

			newValue := reflect.ValueOf(value)
			if newValue.Type().AssignableTo(field.Type()) {
				field.Set(newValue)
			} else {
				convertedValue, err := convertValue(value, field.Type())
				if err != nil {
					return fmt.Errorf("cannot convert value: %w", err)
				}
				field.Set(reflect.ValueOf(convertedValue))
			}
		} else if current.Kind() == reflect.Map {
			if current.IsNil() {
				current.Set(reflect.MakeMap(current.Type()))
			}
			key := reflect.ValueOf(lastPart)
			val := reflect.ValueOf(value)

			elemType := current.Type().Elem()
			if val.Type().AssignableTo(elemType) {
				current.SetMapIndex(key, val)
			} else {
				convertedValue, err := convertValue(value, elemType)
				if err != nil {
					return fmt.Errorf("cannot convert value: %w", err)
				}
				current.SetMapIndex(key, reflect.ValueOf(convertedValue))
			}
		} else {
			return fmt.Errorf("cannot set value on type %s", current.Kind())
		}

		return nil
	}

	var current reflect.Value = reflect.ValueOf(config)

	for i := 0; i < len(parts)-1; i++ {
		part := parts[i]

		if current.Kind() == reflect.Ptr {
			if current.IsNil() {
				current.Set(reflect.New(current.Type().Elem()))
			}
			current = current.Elem()
		}

		switch current.Kind() {
		case reflect.Struct:
			field := findFieldByPath(current, part)
			if !field.IsValid() {
				return fmt.Errorf("field not found: %s", part)
			}
			if !field.CanSet() {
				return fmt.Errorf("field not settable: %s", part)
			}
			current = field

		case reflect.Map:
			if current.IsNil() {
				current.Set(reflect.MakeMap(current.Type()))
			}

			key := reflect.ValueOf(part)
			mapValue := current.MapIndex(key)

			if !mapValue.IsValid() {
				elemType := current.Type().Elem()
				if elemType.Kind() == reflect.Ptr {
					newElem := reflect.New(elemType.Elem())
					current.SetMapIndex(key, newElem)
					current = newElem
				} else {
					newElem := reflect.New(elemType)
					current.SetMapIndex(key, newElem.Elem())
					current = newElem
				}
			} else {
				if mapValue.Kind() == reflect.Ptr {
					current = mapValue
				} else {
					newElem := reflect.New(mapValue.Type())
					newElem.Elem().Set(mapValue)
					current = newElem
				}
			}

		default:
			return fmt.Errorf("cannot navigate into type %s at %s", current.Kind(), part)
		}
	}

	lastPart := parts[len(parts)-1]

	if current.Kind() == reflect.Ptr {
		if current.IsNil() {
			current.Set(reflect.New(current.Type().Elem()))
		}
		current = current.Elem()
	}

	switch current.Kind() {
	case reflect.Struct:
		field := findFieldByPath(current, lastPart)
		if !field.IsValid() {
			return fmt.Errorf("field not found: %s", lastPart)
		}
		if !field.CanSet() {
			return fmt.Errorf("field not settable: %s", lastPart)
		}

		newValue := reflect.ValueOf(value)
		if newValue.Type().AssignableTo(field.Type()) {
			field.Set(newValue)
		} else {
			convertedValue, err := convertValue(value, field.Type())
			if err != nil {
				return fmt.Errorf("cannot convert value: %w", err)
			}
			field.Set(reflect.ValueOf(convertedValue))
		}

	case reflect.Map:
		if current.IsNil() {
			current.Set(reflect.MakeMap(current.Type()))
		}
		key := reflect.ValueOf(lastPart)
		val := reflect.ValueOf(value)

		elemType := current.Type().Elem()
		if val.Type().AssignableTo(elemType) {
			current.SetMapIndex(key, val)
		} else {
			convertedValue, err := convertValue(value, elemType)
			if err != nil {
				return fmt.Errorf("cannot convert value: %w", err)
			}
			current.SetMapIndex(key, reflect.ValueOf(convertedValue))
		}

	default:
		return fmt.Errorf("cannot set value on type %s", current.Kind())
	}

	return nil
}

func convertValue(value interface{}, targetType reflect.Type) (interface{}, error) {
	if targetType.Kind() == reflect.Ptr {
		targetType = targetType.Elem()
	}

	strValue, ok := value.(string)
	if !ok {
		return nil, fmt.Errorf("value must be string for conversion")
	}

	switch targetType.Kind() {
	case reflect.String:
		return strValue, nil
	case reflect.Bool:
		return strconv.ParseBool(strValue)
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		intVal, err := strconv.ParseInt(strValue, 10, 64)
		if err != nil {
			return nil, err
		}
		return reflect.ValueOf(intVal).Convert(targetType).Interface(), nil
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		uintVal, err := strconv.ParseUint(strValue, 10, 64)
		if err != nil {
			return nil, err
		}
		return reflect.ValueOf(uintVal).Convert(targetType).Interface(), nil
	case reflect.Float32, reflect.Float64:
		floatVal, err := strconv.ParseFloat(strValue, 64)
		if err != nil {
			return nil, err
		}
		return reflect.ValueOf(floatVal).Convert(targetType).Interface(), nil
	default:
		return nil, fmt.Errorf("unsupported type conversion to %s", targetType.Kind())
	}
}
