// embed_scripts.go extracts the Python helper scripts embedded in this
// binary (audio_engine.go/wake_word.go's //go:embed directives) to a real
// file on disk the first time they're needed, so they can be exec'd by an
// absolute path regardless of the running process's working directory or
// install location.
package voice

import (
	"fmt"
	"os"
	"path/filepath"
)

// extractScript writes content to name under a jarvis-voice-scripts
// directory in the OS temp dir, overwriting any previous copy (so a
// binary upgrade always ships the script it was built with), and returns
// the resulting absolute path.
func extractScript(name string, content []byte) (string, error) {
	dir := filepath.Join(os.TempDir(), "jarvis-voice-scripts")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("voice: create script directory: %w", err)
	}

	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, content, 0o644); err != nil {
		return "", fmt.Errorf("voice: write embedded script %s: %w", name, err)
	}
	return path, nil
}
