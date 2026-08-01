// Package logger implements SPEC-0005: centralized structured logging.
// Every entry carries a timestamp, component, message, and optional
// metadata, and is written as a single line of JSON.
package logger

import (
	"encoding/json"
	"io"
	"os"
	"sync"
	"time"
)

// Entry is a single structured log record.
type Entry struct {
	Timestamp time.Time      `json:"timestamp"`
	Level     Level          `json:"level"`
	Component string         `json:"component"`
	Message   string         `json:"message"`
	Metadata  map[string]any `json:"metadata,omitempty"`
}

// Logger writes structured, component-scoped log entries to an underlying
// writer. It is safe for concurrent use.
type Logger struct {
	mu        sync.Mutex
	component string
	minLevel  Level
	out       io.Writer
}

// Option configures a Logger created by New.
type Option func(*Logger)

// WithOutput sets the writer entries are written to. Defaults to os.Stdout.
func WithOutput(w io.Writer) Option {
	return func(l *Logger) { l.out = w }
}

// WithMinLevel suppresses entries below the given level. Defaults to
// LevelDebug (nothing suppressed).
func WithMinLevel(min Level) Option {
	return func(l *Logger) { l.minLevel = min }
}

// New creates a Logger that tags every entry with the given component name.
func New(component string, opts ...Option) *Logger {
	l := &Logger{
		component: component,
		minLevel:  LevelDebug,
		out:       os.Stdout,
	}
	for _, opt := range opts {
		opt(l)
	}
	return l
}

// Debug logs a DEBUG-level entry with optional metadata.
func (l *Logger) Debug(message string, metadata map[string]any) {
	l.log(LevelDebug, message, metadata)
}

// Info logs an INFO-level entry with optional metadata.
func (l *Logger) Info(message string, metadata map[string]any) {
	l.log(LevelInfo, message, metadata)
}

// Warn logs a WARN-level entry with optional metadata.
func (l *Logger) Warn(message string, metadata map[string]any) {
	l.log(LevelWarn, message, metadata)
}

// Error logs an ERROR-level entry with optional metadata.
func (l *Logger) Error(message string, metadata map[string]any) {
	l.log(LevelError, message, metadata)
}

func (l *Logger) log(level Level, message string, metadata map[string]any) {
	if !level.enabled(l.minLevel) {
		return
	}

	entry := Entry{
		Timestamp: time.Now().UTC(),
		Level:     level,
		Component: l.component,
		Message:   message,
		Metadata:  metadata,
	}

	data, err := json.Marshal(entry)
	if err != nil {
		// Marshal only fails here if metadata contains an unencodable value
		// (e.g. a channel or func); fall back to a minimal entry rather than
		// dropping the log line.
		data, _ = json.Marshal(Entry{
			Timestamp: entry.Timestamp,
			Level:     level,
			Component: l.component,
			Message:   message,
		})
	}

	l.mu.Lock()
	defer l.mu.Unlock()
	l.out.Write(append(data, '\n'))
}
