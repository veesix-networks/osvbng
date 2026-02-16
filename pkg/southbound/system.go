package southbound

import (
	"context"

	"github.com/veesix-networks/osvbng/pkg/models/system"
)

type System interface {
	GetVersion(ctx context.Context) (string, error)
	GetSystemThreads() ([]system.Thread, error)
}
