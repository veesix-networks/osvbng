package paths

import (
	"fmt"
	"strings"
)

type Path string

func (p Path) String() string {
	return string(p)
}

func (p Path) ExtractWildcards(path string, expectedCount int) ([]string, error) {
	patternParts := strings.Split(string(p), ".")
	pathParts := strings.Split(path, ".")

	if len(patternParts) != len(pathParts) {
		return nil, fmt.Errorf("path format mismatch")
	}

	wildcards := make([]string, 0, expectedCount)
	for i := range patternParts {
		if patternParts[i] == "*" {
			wildcards = append(wildcards, pathParts[i])
		}
	}

	if len(wildcards) != expectedCount {
		return nil, fmt.Errorf("expected %d wildcards, got %d", expectedCount, len(wildcards))
	}

	return wildcards, nil
}
