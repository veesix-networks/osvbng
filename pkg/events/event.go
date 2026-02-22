package events

import "time"

type Event struct {
	ID        string
	Type      string
	Timestamp time.Time
	Source    string
	Data      any
}
