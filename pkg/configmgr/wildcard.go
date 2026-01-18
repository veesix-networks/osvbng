package configmgr

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/veesix-networks/osvbng/pkg/config"
)

func ResolveWildcardKeys(config *config.Config, pattern string) ([][]string, error) {
	parts := strings.Split(pattern, ".")

	var results [][]string
	var currentValues []string

	err := resolveWildcardsRecursive(reflect.ValueOf(config).Elem(), parts, 0, currentValues, &results)
	if err != nil {
		return nil, err
	}

	return results, nil
}

func resolveWildcardsRecursive(current reflect.Value, parts []string, partIndex int, currentValues []string, results *[][]string) error {
	if partIndex >= len(parts) {
		if len(currentValues) > 0 {
			valueCopy := make([]string, len(currentValues))
			copy(valueCopy, currentValues)
			*results = append(*results, valueCopy)
		}
		return nil
	}

	part := parts[partIndex]
	isWildcard := strings.HasPrefix(part, "<") && strings.HasSuffix(part, ">")

	if isWildcard {
		if current.Kind() == reflect.Map {
			for _, key := range current.MapKeys() {
				keyStr := fmt.Sprintf("%v", key.Interface())
				newValues := append(currentValues, keyStr)

				if partIndex == len(parts)-1 {
					valueCopy := make([]string, len(newValues))
					copy(valueCopy, newValues)
					*results = append(*results, valueCopy)
				} else {
					mapValue := current.MapIndex(key)
					if mapValue.IsValid() && !mapValue.IsNil() {
						if mapValue.Kind() == reflect.Ptr {
							mapValue = mapValue.Elem()
						}
						if err := resolveWildcardsRecursive(mapValue, parts, partIndex+1, newValues, results); err != nil {
							return err
						}
					}
				}
			}
		}
		return nil
	}

	if current.Kind() == reflect.Struct {
		field := current.FieldByNameFunc(func(name string) bool {
			return strings.EqualFold(name, part)
		})

		if !field.IsValid() {
			return nil
		}

		if field.Kind() == reflect.Ptr {
			if field.IsNil() {
				return nil
			}
			field = field.Elem()
		}

		return resolveWildcardsRecursive(field, parts, partIndex+1, currentValues, results)
	}

	return nil
}
