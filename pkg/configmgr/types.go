package configmgr

import (
	"time"
)

type ValidationError struct {
	Path     string
	Code     string
	Message  string
	Severity string
}

type ConfigVersion struct {
	Version   int
	Timestamp time.Time
	Config    interface{}
	Changes   []Change
	CommitMsg string
}

type Change struct {
	Type  string
	Path  string
	Value interface{}
}

type DiffResult struct {
	Added    []ConfigLine
	Deleted  []ConfigLine
	Modified []ConfigLine
}

type ConfigLine struct {
	Path  string
	Value string
}

type Command struct {
	Type string
	Func func() error
}
