package sf

import (
	"path/filepath"
	"testing"
	"time"
)

// isolateConfig points configDir() at a temp location for the duration of a test
// by overriding the env vars os.UserConfigDir consults (HOME on macOS,
// XDG_CONFIG_HOME/HOME on Linux).
func isolateConfig(t *testing.T) {
	t.Helper()
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(tmp, "xdg"))
}

func TestSObjectDiskCacheRoundTrip(t *testing.T) {
	isolateConfig(t)
	org := "admin@example.com"
	in := []SObjectSummary{
		{Name: "Account", Label: "Account", Queryable: true},
		{Name: "Contact", Label: "Contact", Queryable: true, Custom: false},
	}

	if _, _, ok := readSObjectCache(org); ok {
		t.Fatal("expected a cache miss before any write")
	}

	writeSObjectCache(org, in)
	got, at, ok := readSObjectCache(org)
	if !ok {
		t.Fatal("expected a cache hit after write")
	}
	if !cacheFresh(at) {
		t.Errorf("freshly written cache should be fresh, got cachedAt %v", at)
	}
	if len(got) != len(in) || got[0].Name != "Account" || got[1].Name != "Contact" {
		t.Errorf("round-trip mismatch: got %+v", got)
	}
}

func TestDescribeDiskCacheRoundTrip(t *testing.T) {
	isolateConfig(t)
	org := "admin@example.com"
	d := Describe{
		Name:  "Account",
		Label: "Account",
		Fields: []Field{
			{Name: "Id", Type: "id"},
			{Name: "Name", Type: "string", Length: 255},
		},
	}

	writeDescribeCache(org, d)

	// Lookup is case-insensitive, matching the in-memory cache keying.
	got, at, ok := readDescribeCache(org, "account")
	if !ok {
		t.Fatal("expected a describe cache hit (case-insensitive)")
	}
	if !cacheFresh(at) {
		t.Error("freshly written describe should be fresh")
	}
	if got.Name != "Account" || len(got.Fields) != 2 {
		t.Errorf("describe round-trip mismatch: got %+v", got)
	}
}

func TestDescribeCacheScopedPerOrg(t *testing.T) {
	isolateConfig(t)
	writeDescribeCache("orgA", Describe{Name: "Account", Fields: []Field{{Name: "Id"}}})

	if _, _, ok := readDescribeCache("orgB", "Account"); ok {
		t.Error("orgB must not see orgA's cached describe")
	}
	if _, _, ok := readDescribeCache("orgA", "Account"); !ok {
		t.Error("orgA should still see its own cached describe")
	}
}

func TestCacheFreshTTL(t *testing.T) {
	if cacheFresh(time.Time{}) {
		t.Error("zero time must not be considered fresh")
	}
	if !cacheFresh(time.Now()) {
		t.Error("now must be fresh")
	}
	if cacheFresh(time.Now().Add(-DescribeCacheTTL - time.Minute)) {
		t.Error("a copy older than the TTL must be stale")
	}
}

func TestOrgDirNameAvoidsCollision(t *testing.T) {
	// Distinct orgs that sanitize to the same string must still get distinct dirs.
	if orgDirName("a@b") == orgDirName("a_b") {
		t.Error("orgDirName collided for a@b and a_b")
	}
	// Stable for the same input.
	if orgDirName("a@b") != orgDirName("a@b") {
		t.Error("orgDirName is not stable for identical input")
	}
}

func TestSanitizeFileName(t *testing.T) {
	cases := map[string]string{
		"Account":         "Account",
		"My_Object__c":    "My_Object__c",
		"admin@a.com":     "admin_a.com",
		"weird/../name":   "weird_.._name",
		"":                "org",
		"spaces and #!$@": "spaces_and_____",
	}
	for in, want := range cases {
		if got := sanitizeFileName(in); got != want {
			t.Errorf("sanitizeFileName(%q) = %q, want %q", in, got, want)
		}
	}
}
