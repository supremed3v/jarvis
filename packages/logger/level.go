package logger

import (
	"fmt"
	"strings"
)

// Level identifies the severity of a log entry, per SPEC-0005.
type Level string

const (
	LevelDebug Level = "DEBUG"
	LevelInfo  Level = "INFO"
	LevelWarn  Level = "WARN"
	LevelError Level = "ERROR"
)

// severity orders levels from least to most severe, so a minimum level can
// filter out anything below it.
var severity = map[Level]int{
	LevelDebug: 0,
	LevelInfo:  1,
	LevelWarn:  2,
	LevelError: 3,
}

// ParseLevel converts a case-insensitive level name (e.g. from config or an
// environment variable) into a Level. It returns an error for unknown names
// rather than silently defaulting, so misconfiguration is never masked.
func ParseLevel(name string) (Level, error) {
	switch strings.ToUpper(name) {
	case "DEBUG":
		return LevelDebug, nil
	case "INFO":
		return LevelInfo, nil
	case "WARN", "WARNING":
		return LevelWarn, nil
	case "ERROR":
		return LevelError, nil
	default:
		return "", fmt.Errorf("logger: unknown level %q", name)
	}
}

func (l Level) enabled(min Level) bool {
	return severity[l] >= severity[min]
}
