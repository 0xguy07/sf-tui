package sf

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

var unsafeFilename = regexp.MustCompile(`[^A-Za-z0-9._-]+`)

func ExportDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(home, "sf-tui-queries")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	return dir, nil
}

// ExportSOQL writes `soql` to a file. If name is empty, uses a timestamp.
// Returns the full path written.
func ExportSOQL(name, soql string) (string, error) {
	dir, err := ExportDir()
	if err != nil {
		return "", err
	}
	base := strings.TrimSpace(name)
	if base == "" {
		base = "query-" + time.Now().Format("2006-01-02-150405")
	}
	base = unsafeFilename.ReplaceAllString(base, "-")
	if !strings.HasSuffix(strings.ToLower(base), ".soql") {
		base += ".soql"
	}
	path := filepath.Join(dir, base)
	content := strings.TrimSpace(soql) + "\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return "", fmt.Errorf("write %s: %w", path, err)
	}
	return path, nil
}
