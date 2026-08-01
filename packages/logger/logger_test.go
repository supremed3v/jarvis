package logger

import (
	"bytes"
	"encoding/json"
	"strings"
	"sync"
	"testing"
)

func TestLogger_LevelsIncludeRequiredFields(t *testing.T) {
	var buf bytes.Buffer
	log := New("core-runtime", WithOutput(&buf))

	log.Info("started", map[string]any{"port": 8080})

	var entry Entry
	if err := json.Unmarshal(buf.Bytes(), &entry); err != nil {
		t.Fatalf("Unmarshal(entry) returned error: %v, raw=%s", err, buf.String())
	}
	if entry.Timestamp.IsZero() {
		t.Errorf("Timestamp is zero, want set")
	}
	if entry.Level != LevelInfo {
		t.Errorf("Level = %q, want %q", entry.Level, LevelInfo)
	}
	if entry.Component != "core-runtime" {
		t.Errorf("Component = %q, want %q", entry.Component, "core-runtime")
	}
	if entry.Message != "started" {
		t.Errorf("Message = %q, want %q", entry.Message, "started")
	}
	if entry.Metadata["port"] != float64(8080) {
		t.Errorf("Metadata[port] = %v, want 8080", entry.Metadata["port"])
	}
}

func TestLogger_AllFourLevelsProduceCorrectTag(t *testing.T) {
	var buf bytes.Buffer
	log := New("test", WithOutput(&buf))

	log.Debug("d", nil)
	log.Info("i", nil)
	log.Warn("w", nil)
	log.Error("e", nil)

	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	if len(lines) != 4 {
		t.Fatalf("got %d lines, want 4: %v", len(lines), lines)
	}

	want := []Level{LevelDebug, LevelInfo, LevelWarn, LevelError}
	for i, line := range lines {
		var entry Entry
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			t.Fatalf("Unmarshal(line %d) returned error: %v", i, err)
		}
		if entry.Level != want[i] {
			t.Errorf("line %d Level = %q, want %q", i, entry.Level, want[i])
		}
	}
}

func TestLogger_MetadataOmittedWhenNil(t *testing.T) {
	var buf bytes.Buffer
	log := New("test", WithOutput(&buf))

	log.Info("started", nil)

	if strings.Contains(buf.String(), "metadata") {
		t.Errorf("output contains metadata field when none was given: %s", buf.String())
	}
}

func TestLogger_MinLevelSuppressesLowerSeverity(t *testing.T) {
	var buf bytes.Buffer
	log := New("test", WithOutput(&buf), WithMinLevel(LevelWarn))

	log.Debug("suppressed", nil)
	log.Info("suppressed", nil)
	log.Warn("kept", nil)
	log.Error("kept", nil)

	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	if len(lines) != 2 {
		t.Fatalf("got %d lines, want 2 (WARN and ERROR only): %v", len(lines), lines)
	}
}

func TestParseLevel(t *testing.T) {
	cases := map[string]Level{
		"debug":   LevelDebug,
		"INFO":    LevelInfo,
		"warn":    LevelWarn,
		"ERROR":   LevelError,
		"Debug":   LevelDebug,
		"Warning": LevelWarn,
	}
	for input, want := range cases {
		got, err := ParseLevel(input)
		if err != nil {
			t.Fatalf("ParseLevel(%q) returned error: %v", input, err)
		}
		if got != want {
			t.Errorf("ParseLevel(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestParseLevel_UnknownFailsSafely(t *testing.T) {
	_, err := ParseLevel("verbose")
	if err == nil {
		t.Fatalf("ParseLevel(%q) returned no error, want error for unknown level", "verbose")
	}
}

func TestLogger_ConcurrentWritesDoNotRace(t *testing.T) {
	var buf bytes.Buffer
	log := New("test", WithOutput(&buf))

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			log.Info("concurrent", map[string]any{"i": i})
		}()
	}
	wg.Wait()

	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	if len(lines) != 50 {
		t.Fatalf("got %d lines, want 50", len(lines))
	}
	for i, line := range lines {
		var entry Entry
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			t.Fatalf("line %d is not valid JSON (interleaved write?): %v, line=%q", i, err, line)
		}
	}
}
