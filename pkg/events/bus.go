package events

import (
	"github.com/veesix-networks/osvbng/pkg/models"
)

type EventHandler func(models.Event) error

type Bus interface {
	Publish(topic string, event models.Event) error
	Subscribe(topic string, handler EventHandler) error
	Unsubscribe(topic string, handler EventHandler)
	Close() error
}
