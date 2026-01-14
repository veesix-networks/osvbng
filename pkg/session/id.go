package session

import (
	"strings"

	"github.com/google/uuid"
)

func GenerateID() string {
	return uuid.New().String()
}

func ToAcctSessionID(sessionID string) string {
	cleaned := strings.ReplaceAll(sessionID, "-", "")
	if len(cleaned) >= 8 {
		return cleaned[:8]
	}
	return cleaned
}
