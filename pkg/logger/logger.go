package logger

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"
	"sync"
)

type LogLevel string

const (
	LogLevelDebug LogLevel = "debug"
	LogLevelInfo  LogLevel = "info"
	LogLevelWarn  LogLevel = "warn"
	LogLevelError LogLevel = "error"
)

var (
	Log             *slog.Logger
	defaultLevel    slog.Level
	componentLevels map[string]slog.Level
	levelsMu        sync.RWMutex
	format          string
	pid             int
	loggerCache     sync.Map
)

func init() {
	defaultLevel = slog.LevelInfo
	componentLevels = make(map[string]slog.Level)
	format = "text"
	pid = os.Getpid()

	handler := NewBNGTextHandler(os.Stdout, nil, "")
	Log = slog.New(handler)
}

func Configure(logFormat string, level LogLevel, components map[string]LogLevel) {
	levelsMu.Lock()
	defaultLevel = parseLevel(string(level))
	format = logFormat
	componentLevels = make(map[string]slog.Level)
	for name, lvl := range components {
		componentLevels[name] = parseLevel(string(lvl))
	}
	levelsMu.Unlock()

	loggerCache = sync.Map{}

	var handler slog.Handler
	if strings.ToLower(format) == "json" {
		handler = slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
			Level: defaultLevel,
		})
	} else {
		handler = NewBNGTextHandler(os.Stdout, nil, "")
	}

	Log = slog.New(handler)
}

type BNGTextHandler struct {
	opts      *slog.HandlerOptions
	mu        sync.Mutex
	w         io.Writer
	attrs     []slog.Attr
	component string
}

func NewBNGTextHandler(w io.Writer, opts *slog.HandlerOptions, component string) *BNGTextHandler {
	if opts == nil {
		opts = &slog.HandlerOptions{}
	}
	return &BNGTextHandler{
		w:         w,
		opts:      opts,
		component: component,
	}
}

func (h *BNGTextHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return level >= getEffectiveLevel(h.component)
}

func (h *BNGTextHandler) Handle(ctx context.Context, r slog.Record) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	attrs := make(map[string]any)

	r.Attrs(func(a slog.Attr) bool {
		attrs[a.Key] = a.Value.Any()
		return true
	})

	for _, a := range h.attrs {
		attrs[a.Key] = a.Value.Any()
	}

	buf := make([]byte, 0, 256)
	buf = append(buf, r.Time.Format("2006/01/02 15:04:05.000")...)
	buf = append(buf, fmt.Sprintf(" [%d]", pid)...)

	if h.component != "" {
		buf = append(buf, fmt.Sprintf(" [%s]", h.component)...)
	}

	buf = append(buf, ' ')
	buf = append(buf, r.Message...)

	for k, v := range attrs {
		buf = append(buf, fmt.Sprintf(" %s=%v", k, v)...)
	}

	buf = append(buf, '\n')
	_, err := h.w.Write(buf)
	return err
}

func (h *BNGTextHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &BNGTextHandler{
		w:         h.w,
		opts:      h.opts,
		attrs:     append(h.attrs, attrs...),
		component: h.component,
	}
}

func (h *BNGTextHandler) WithGroup(name string) slog.Handler {
	newComponent := h.component
	if newComponent != "" {
		newComponent = newComponent + "." + name
	} else {
		newComponent = name
	}
	return &BNGTextHandler{
		w:         h.w,
		opts:      h.opts,
		attrs:     h.attrs,
		component: newComponent,
	}
}

func parseLevel(level string) slog.Level {
	switch strings.ToLower(level) {
	case "debug":
		return slog.LevelDebug
	case "info":
		return slog.LevelInfo
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

func getEffectiveLevel(component string) slog.Level {
	levelsMu.RLock()
	defer levelsMu.RUnlock()

	if level, ok := componentLevels[component]; ok {
		return level
	}

	path := component
	for {
		idx := strings.LastIndex(path, ".")
		if idx < 0 {
			break
		}
		path = path[:idx]
		if level, ok := componentLevels[path]; ok {
			return level
		}
	}

	return defaultLevel
}

type BNGJSONHandler struct {
	inner     *slog.JSONHandler
	component string
}

func newJSONHandler(component string) *BNGJSONHandler {
	return &BNGJSONHandler{
		inner: slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
			Level: slog.LevelDebug,
		}),
		component: component,
	}
}

func (h *BNGJSONHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return level >= getEffectiveLevel(h.component)
}

func (h *BNGJSONHandler) Handle(ctx context.Context, r slog.Record) error {
	if h.component != "" {
		r.AddAttrs(slog.String("component", h.component))
	}
	return h.inner.Handle(ctx, r)
}

func (h *BNGJSONHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &BNGJSONHandler{
		inner:     h.inner.WithAttrs(attrs).(*slog.JSONHandler),
		component: h.component,
	}
}

func (h *BNGJSONHandler) WithGroup(name string) slog.Handler {
	newComponent := h.component
	if newComponent != "" {
		newComponent = newComponent + "." + name
	} else {
		newComponent = name
	}
	return &BNGJSONHandler{
		inner:     h.inner,
		component: newComponent,
	}
}

func Get(name string) *slog.Logger {
	if l, ok := loggerCache.Load(name); ok {
		return l.(*slog.Logger)
	}

	var handler slog.Handler
	if strings.ToLower(format) == "json" {
		handler = newJSONHandler(name)
	} else {
		handler = NewBNGTextHandler(os.Stdout, nil, name)
	}

	l := slog.New(handler)
	loggerCache.Store(name, l)
	return l
}

func SetComponentLevel(name string, level LogLevel) {
	levelsMu.Lock()
	componentLevels[name] = parseLevel(string(level))
	levelsMu.Unlock()
	loggerCache.Delete(name)
}

func ClearComponentLevel(name string) {
	levelsMu.Lock()
	delete(componentLevels, name)
	levelsMu.Unlock()
	loggerCache.Delete(name)
}

func GetComponentLevels() map[string]LogLevel {
	levelsMu.RLock()
	defer levelsMu.RUnlock()
	result := make(map[string]LogLevel)
	for name, level := range componentLevels {
		result[name] = levelToLogLevel(level)
	}
	return result
}

func GetDefaultLevel() LogLevel {
	return levelToLogLevel(defaultLevel)
}

func levelToLogLevel(level slog.Level) LogLevel {
	switch level {
	case slog.LevelDebug:
		return LogLevelDebug
	case slog.LevelInfo:
		return LogLevelInfo
	case slog.LevelWarn:
		return LogLevelWarn
	case slog.LevelError:
		return LogLevelError
	default:
		return LogLevelInfo
	}
}

type SessionAttrs struct {
	SessionID     string
	AcctSessionID string
	MAC           string
	SVLAN         uint16
	CVLAN         uint16
	Protocol      string
	Username      string
}

func WithSession(logger *slog.Logger, attrs SessionAttrs) *slog.Logger {
	args := make([]any, 0, 14)

	if attrs.SessionID != "" {
		args = append(args, "session_id", attrs.SessionID)
	}
	if attrs.AcctSessionID != "" {
		args = append(args, "acct_session_id", attrs.AcctSessionID)
	}
	if attrs.MAC != "" {
		args = append(args, "mac", attrs.MAC)
	}
	if attrs.SVLAN > 0 {
		args = append(args, "svlan", attrs.SVLAN)
	}
	if attrs.CVLAN > 0 {
		args = append(args, "cvlan", attrs.CVLAN)
	}
	if attrs.Protocol != "" {
		args = append(args, "protocol", attrs.Protocol)
	}
	if attrs.Username != "" {
		args = append(args, "username", attrs.Username)
	}

	return logger.With(args...)
}
