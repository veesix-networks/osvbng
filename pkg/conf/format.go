package conf

import (
	"fmt"
	"strings"

	"github.com/veesix-networks/osvbng/pkg/conf/handlers"
	"github.com/veesix-networks/osvbng/pkg/conf/types"
)

func FormatChanges(changes []*handlers.HandlerContext) *types.DiffResult {
	result := &types.DiffResult{}

	for _, change := range changes {
		line := types.ConfigLine{
			Path:  change.Path,
			Value: formatValue(change.NewValue),
		}

		if change.OldValue == nil && change.NewValue != nil {
			result.Added = append(result.Added, line)
		} else if change.OldValue != nil && change.NewValue == nil {
			line.Value = formatValue(change.OldValue)
			result.Deleted = append(result.Deleted, line)
		} else {
			result.Modified = append(result.Modified, line)
		}
	}

	return result
}

func FormatDiff(result *types.DiffResult) string {
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
	case *types.InterfaceConfig:
		return fmt.Sprintf("interface:%s", val.Name)
	case *types.StaticRoute:
		return fmt.Sprintf("%s via %s", val.Destination, val.NextHop)
	default:
		return fmt.Sprintf("%v", val)
	}
}
