package events

type Handler func(Event)

type Subscription interface {
	Unsubscribe()
}

type TopicStats struct {
	Topic       string `json:"topic"`
	Subscribers int    `json:"subscribers"`
}

type Stats struct {
	Topics       []TopicStats `json:"topics"`
	PublishChLen int          `json:"publish-channel-length"`
	PublishChCap int          `json:"publish-channel-capacity"`
	Published    uint64       `json:"published"`
	Dropped      uint64       `json:"dropped"`
	DebugTopics  []string     `json:"debug-topics,omitempty"`
}

type Bus interface {
	Publish(topic string, event Event)
	Subscribe(topic string, handler Handler) Subscription
	SubscribeAll(handler Handler) Subscription
	Stats() Stats
	SetDebugTopics(topics []string)
	DebugTopics() []string
	Close() error
}
