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

var (
	Log             *slog.Logger
	defaultLevel    slog.Level
	componentLevels map[string]slog.Level
	format          string
	pid             int
)

func init() {
	defaultLevel = slog.LevelInfo
	componentLevels = make(map[string]slog.Level)
	format = "text"
	pid = os.Getpid()

	handler := NewBNGTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: defaultLevel,
	})
	Log = slog.New(handler)
}

func Configure(logFormat, level string, components map[string]string) {
	defaultLevel = parseLevel(level)
	format = logFormat

	componentLevels = make(map[string]slog.Level)
	for name, lvl := range components {
		componentLevels[name] = parseLevel(lvl)
	}

	var handler slog.Handler
	if strings.ToLower(format) == "json" {
		handler = slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
			Level: defaultLevel,
		})
	} else {
		handler = NewBNGTextHandler(os.Stdout, &slog.HandlerOptions{
			Level: defaultLevel,
		})
	}

	Log = slog.New(handler)
}

type BNGTextHandler struct {
	opts  *slog.HandlerOptions
	mu    sync.Mutex
	w     io.Writer
	attrs []slog.Attr
	group string
}

func NewBNGTextHandler(w io.Writer, opts *slog.HandlerOptions) *BNGTextHandler {
	if opts == nil {
		opts = &slog.HandlerOptions{}
	}
	return &BNGTextHandler{
		w:    w,
		opts: opts,
	}
}

func (h *BNGTextHandler) Enabled(ctx context.Context, level slog.Level) bool {
	minLevel := slog.LevelInfo
	if h.opts.Level != nil {
		minLevel = h.opts.Level.Level()
	}
	return level >= minLevel
}

func (h *BNGTextHandler) Handle(ctx context.Context, r slog.Record) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	var component string
	attrs := make(map[string]any)

	r.Attrs(func(a slog.Attr) bool {
		if a.Key == "component" {
			component = a.Value.String()
		} else {
			attrs[a.Key] = a.Value.Any()
		}
		return true
	})

	for _, a := range h.attrs {
		if a.Key == "component" {
			component = a.Value.String()
		} else {
			attrs[a.Key] = a.Value.Any()
		}
	}

	buf := make([]byte, 0, 256)
	buf = append(buf, r.Time.Format("2006/01/02 15:04:05.000")...)
	buf = append(buf, fmt.Sprintf(" [%d]", pid)...)

	if component != "" {
		buf = append(buf, fmt.Sprintf(" [%s]", component)...)
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
		w:     h.w,
		opts:  h.opts,
		attrs: append(h.attrs, attrs...),
		group: h.group,
	}
}

func (h *BNGTextHandler) WithGroup(name string) slog.Handler {
	return &BNGTextHandler{
		w:     h.w,
		opts:  h.opts,
		attrs: h.attrs,
		group: name,
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

func Component(name string) *slog.Logger {
	level, ok := componentLevels[name]
	if !ok {
		level = defaultLevel
	}

	var handler slog.Handler
	if strings.ToLower(format) == "json" {
		handler = slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
			Level: level,
		})
	} else {
		handler = NewBNGTextHandler(os.Stdout, &slog.HandlerOptions{
			Level: level,
		})
	}

	return slog.New(handler).With("component", name)
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
