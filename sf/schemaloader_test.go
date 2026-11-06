package sf

import (
	"strings"
	"testing"
)

func isolate(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("PATH", dir) // no sf on the path
	t.Setenv("HOME", dir)
	t.Setenv("XDG_CONFIG_HOME", dir+"/.config")
}

func TestFetchSObjects_SfMissing(t *testing.T) {
	isolate(t)
	_, cached, _, err := fetchSObjects("x", true)
	if err == nil || !strings.Contains(err.Error(), "executable file not found") || cached {
		t.Errorf("err=%v cached=%v", err, cached)
	}
}

func TestFetchDescribe_SfMissing(t *testing.T) {
	isolate(t)
	_, cached, _, err := fetchDescribe("x", "Account", true)
	if err == nil || !strings.Contains(err.Error(), "executable file not found") || cached {
		t.Errorf("err=%v cached=%v", err, cached)
	}
}

func TestDefaultSchemaLoader_Implements(t *testing.T) {
	var l SchemaLoader = DefaultSchemaLoader()
	if l == nil {
		t.Fatal("nil loader")
	}
}
