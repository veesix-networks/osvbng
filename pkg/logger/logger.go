// Copyright 2025 Veesix Networks Ltd
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package logger

import (
	"fmt"
	"io"
	"log/slog"
	"net"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/diode"
)

type LogLevel string

const (
	LogLevelDebug LogLevel = "debug"
	LogLevelInfo  LogLevel = "info"
	LogLevelWarn  LogLevel = "warn"
	LogLevelError LogLevel = "error"
)

type Logger struct {
	zl        zerolog.Logger
	component string
}

type levelMap = map[string]zerolog.Level

var (
	Log             *Logger
	defaultLevel    zerolog.Level
	componentLevels atomic.Pointer[levelMap]
	loggerCache     sync.Map
	diodeWriter     *diode.Writer
)

func init() {
	defaultLevel = zerolog.InfoLevel
	m := make(levelMap)
	componentLevels.Store(&m)

	zerolog.TimeFieldFormat = "2006/01/02 15:04:05.000"

	w := zerolog.ConsoleWriter{
		Out:        os.Stdout,
		TimeFormat: "2006/01/02 15:04:05.000",
		NoColor:    true,
	}
	Log = &Logger{
		zl: zerolog.New(w).With().Timestamp().Logger(),
	}
}

func Configure(logFormat string, level LogLevel, components map[string]LogLevel) {
	defaultLevel = parseZerologLevel(string(level))

	m := make(levelMap, len(components))
	for name, lvl := range components {
		m[name] = parseZerologLevel(string(lvl))
	}
	componentLevels.Store(&m)

	loggerCache = sync.Map{}

	var w io.Writer
	if strings.ToLower(logFormat) == "json" {
		dw := diode.NewWriter(os.Stdout, 100000, 10*time.Millisecond, func(missed int) {
			fmt.Fprintf(os.Stderr, "WARN: logger dropped %d messages\n", missed)
		})
		diodeWriter = &dw
		w = diodeWriter
	} else {
		cw := zerolog.ConsoleWriter{
			Out:        os.Stdout,
			TimeFormat: "2006/01/02 15:04:05.000",
			NoColor:    true,
		}
		dw := diode.NewWriter(cw, 100000, 10*time.Millisecond, func(missed int) {
			fmt.Fprintf(os.Stderr, "WARN: logger dropped %d messages\n", missed)
		})
		diodeWriter = &dw
		w = diodeWriter
	}

	Log = &Logger{
		zl: zerolog.New(w).With().Timestamp().Logger(),
	}
}

func Sync() {
	if diodeWriter != nil {
		diodeWriter.Close()
	}
}

func (l *Logger) enabled(level zerolog.Level) bool {
	levels := *componentLevels.Load()

	if lvl, ok := levels[l.component]; ok {
		return level >= lvl
	}

	path := l.component
	for {
		idx := strings.LastIndex(path, ".")
		if idx < 0 {
			break
		}
		path = path[:idx]
		if lvl, ok := levels[path]; ok {
			return level >= lvl
		}
	}

	return level >= defaultLevel
}

func (l *Logger) Info(msg string, args ...any) {
	if !l.enabled(zerolog.InfoLevel) {
		return
	}
	logEvent(l.zl.Info(), msg, args)
}

func (l *Logger) Debug(msg string, args ...any) {
	if !l.enabled(zerolog.DebugLevel) {
		return
	}
	logEvent(l.zl.Debug(), msg, args)
}

func (l *Logger) Error(msg string, args ...any) {
	logEvent(l.zl.Error(), msg, args)
}

func (l *Logger) Warn(msg string, args ...any) {
	logEvent(l.zl.Warn(), msg, args)
}

func (l *Logger) With(args ...any) *Logger {
	zctx := l.zl.With()
	for i := 0; i < len(args); i++ {
		switch v := args[i].(type) {
		case slog.Attr:
			zctx = addContext(zctx, v.Key, v.Value.Any())
		case string:
			if i+1 < len(args) {
				i++
				zctx = addContext(zctx, v, args[i])
			}
		}
	}
	return &Logger{zl: zctx.Logger(), component: l.component}
}

func (l *Logger) WithGroup(name string) *Logger {
	component := l.component
	if component != "" {
		component = component + "." + name
	} else {
		component = name
	}
	return &Logger{
		zl:        l.zl.With().Str("component", component).Logger(),
		component: component,
	}
}

func logEvent(e *zerolog.Event, msg string, args []any) {
	for i := 0; i < len(args); i++ {
		switch v := args[i].(type) {
		case slog.Attr:
			e = addField(e, v.Key, v.Value.Any())
		case string:
			if i+1 < len(args) {
				i++
				e = addField(e, v, args[i])
			}
		}
	}
	e.Msg(msg)
}

func addField(e *zerolog.Event, key string, val any) *zerolog.Event {
	switch v := val.(type) {
	case string:
		return e.Str(key, v)
	case int:
		return e.Int(key, v)
	case int64:
		return e.Int64(key, v)
	case uint:
		return e.Uint(key, v)
	case uint8:
		return e.Uint8(key, v)
	case uint16:
		return e.Uint16(key, v)
	case uint32:
		return e.Uint32(key, v)
	case uint64:
		return e.Uint64(key, v)
	case float64:
		return e.Float64(key, v)
	case bool:
		return e.Bool(key, v)
	case error:
		return e.AnErr(key, v)
	case net.IP:
		return e.IPAddr(key, v)
	case net.HardwareAddr:
		return e.Str(key, v.String())
	case time.Duration:
		return e.Dur(key, v)
	case time.Time:
		return e.Time(key, v)
	case fmt.Stringer:
		return e.Stringer(key, v)
	default:
		return e.Interface(key, v)
	}
}

func addContext(zctx zerolog.Context, key string, val any) zerolog.Context {
	switch v := val.(type) {
	case string:
		return zctx.Str(key, v)
	case int:
		return zctx.Int(key, v)
	case int64:
		return zctx.Int64(key, v)
	case uint16:
		return zctx.Uint16(key, v)
	case uint32:
		return zctx.Uint32(key, v)
	case uint64:
		return zctx.Uint64(key, v)
	case bool:
		return zctx.Bool(key, v)
	case error:
		return zctx.AnErr(key, v)
	case net.IP:
		return zctx.IPAddr(key, v)
	case net.HardwareAddr:
		return zctx.Str(key, v.String())
	case fmt.Stringer:
		return zctx.Stringer(key, v)
	default:
		return zctx.Interface(key, v)
	}
}

func Get(name string) *Logger {
	if l, ok := loggerCache.Load(name); ok {
		return l.(*Logger)
	}

	l := &Logger{
		zl:        Log.zl.With().Str("component", name).Logger(),
		component: name,
	}
	loggerCache.Store(name, l)
	return l
}

func NewTest() *Logger {
	return &Logger{
		zl:        zerolog.New(io.Discard),
		component: "test",
	}
}

func SetComponentLevel(name string, level LogLevel) {
	old := *componentLevels.Load()
	newMap := make(levelMap, len(old)+1)
	for k, v := range old {
		newMap[k] = v
	}
	newMap[name] = parseZerologLevel(string(level))
	componentLevels.Store(&newMap)
	loggerCache.Delete(name)
}

func ClearComponentLevel(name string) {
	old := *componentLevels.Load()
	newMap := make(levelMap, len(old))
	for k, v := range old {
		if k != name {
			newMap[k] = v
		}
	}
	componentLevels.Store(&newMap)
	loggerCache.Delete(name)
}

func GetComponentLevels() map[string]LogLevel {
	levels := *componentLevels.Load()
	result := make(map[string]LogLevel, len(levels))
	for name, level := range levels {
		result[name] = zerologToLogLevel(level)
	}
	return result
}

func GetDefaultLevel() LogLevel {
	return zerologToLogLevel(defaultLevel)
}

func parseZerologLevel(level string) zerolog.Level {
	switch strings.ToLower(level) {
	case "debug":
		return zerolog.DebugLevel
	case "info":
		return zerolog.InfoLevel
	case "warn", "warning":
		return zerolog.WarnLevel
	case "error":
		return zerolog.ErrorLevel
	default:
		return zerolog.InfoLevel
	}
}

func zerologToLogLevel(level zerolog.Level) LogLevel {
	switch level {
	case zerolog.DebugLevel:
		return LogLevelDebug
	case zerolog.InfoLevel:
		return LogLevelInfo
	case zerolog.WarnLevel:
		return LogLevelWarn
	case zerolog.ErrorLevel:
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

func WithSession(l *Logger, attrs SessionAttrs) *Logger {
	zctx := l.zl.With()

	if attrs.SessionID != "" {
		zctx = zctx.Str("session_id", attrs.SessionID)
	}
	if attrs.AcctSessionID != "" {
		zctx = zctx.Str("acct_session_id", attrs.AcctSessionID)
	}
	if attrs.MAC != "" {
		zctx = zctx.Str("mac", attrs.MAC)
	}
	if attrs.SVLAN > 0 {
		zctx = zctx.Uint16("svlan", attrs.SVLAN)
	}
	if attrs.CVLAN > 0 {
		zctx = zctx.Uint16("cvlan", attrs.CVLAN)
	}
	if attrs.Protocol != "" {
		zctx = zctx.Str("protocol", attrs.Protocol)
	}
	if attrs.Username != "" {
		zctx = zctx.Str("username", attrs.Username)
	}

	return &Logger{zl: zctx.Logger(), component: l.component}
}
