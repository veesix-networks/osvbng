package configmgr

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"

	"github.com/veesix-networks/osvbng/pkg/config/interfaces"
	"github.com/veesix-networks/osvbng/pkg/config/protocols"
	"github.com/veesix-networks/osvbng/pkg/handlers/conf"
)

func FormatChanges(changes []*conf.HandlerContext) *DiffResult {
	result := &DiffResult{}

	for _, change := range changes {
		line := ConfigLine{
			Path:  change.Path,
			Value: formatValue(change.NewValue),
		}

		switch {
		case change.OldValue == nil && change.NewValue != nil:
			result.Added = append(result.Added, line)
		case change.OldValue != nil && change.NewValue == nil:
			line.Value = formatValue(change.OldValue)
			result.Deleted = append(result.Deleted, line)
		case reflect.DeepEqual(change.OldValue, change.NewValue):
			// No-op: candidate session walks every leaf when applying the
			// loaded YAML, so on a restart with unchanged config every field
			// arrives here with OldValue == NewValue. Skip; these aren't
			// modifications.
		default:
			result.Modified = append(result.Modified, line)
		}
	}

	return result
}

// IsEmpty reports whether the diff has zero changes across all three buckets.
func (r *DiffResult) IsEmpty() bool {
	return len(r.Added) == 0 && len(r.Modified) == 0 && len(r.Deleted) == 0
}

func FormatDiff(result *DiffResult) string {
	var sb strings.Builder

	if len(result.Added) > 0 {
		sb.WriteString("Added:\n")
		for _, line := range result.Added {
			sb.WriteString(fmt.Sprintf("  + %s = %s\n", line.Path, line.Value))
		}
	}

	if len(result.Modified) > 0 {
		sb.WriteString("Modified:\n")
		for _, line := range result.Modified {
			sb.WriteString(fmt.Sprintf("  ~ %s = %s\n", line.Path, line.Value))
		}
	}

	if len(result.Deleted) > 0 {
		sb.WriteString("Deleted:\n")
		for _, line := range result.Deleted {
			sb.WriteString(fmt.Sprintf("  - %s = %s\n", line.Path, line.Value))
		}
	}

	if len(result.Added) == 0 && len(result.Modified) == 0 && len(result.Deleted) == 0 {
		sb.WriteString("No changes\n")
	}

	return sb.String()
}

func formatValue(v interface{}) string {
	if v == nil {
		return ""
	}

	switch val := v.(type) {
	case string:
		return val
	case bool:
		return fmt.Sprintf("%v", val)
	case int:
		return fmt.Sprintf("%d", val)
	case *interfaces.InterfaceConfig:
		return fmt.Sprintf("interface:%s", val.Name)
	case *protocols.StaticRoute:
		return fmt.Sprintf("%s via %s", val.Destination, val.NextHop)
	}

	// Deref pointers and fall back to JSON for anything structured; produces
	// `{"enabled":false}` etc. instead of Go's `&{false}` default which hides
	// field names and prints pointer addresses for embedded pointers.
	rv := reflect.ValueOf(v)
	for rv.Kind() == reflect.Pointer {
		if rv.IsNil() {
			return ""
		}
		rv = rv.Elem()
	}
	switch rv.Kind() {
	case reflect.Struct, reflect.Map, reflect.Slice, reflect.Array:
		if data, err := json.Marshal(rv.Interface()); err == nil {
			return string(data)
		}
	}
	return fmt.Sprintf("%v", rv.Interface())
}
