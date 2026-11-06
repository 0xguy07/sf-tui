package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The non-interactive commands are read-only by construction: nothing in
// their files may reference the write gate.
func TestCLIFilesNeverReferenceGate(t *testing.T) {
	files, _ := filepath.Glob("cli_*.go")
	more, _ := filepath.Glob("*_text.go")
	for _, f := range append(files, more...) {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(strings.ToLower(string(b)), "gate") {
			t.Errorf("%s references the write gate", f)
		}
	}
}
