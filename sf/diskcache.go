package sf

import (
	"encoding/json"
	"fmt"
	"hash/fnv"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// DescribeCacheTTL bounds how long a persisted sobject list or describe is served
// from disk before sf-tui refetches it from the org. Schema changes rarely within
// a working day; ctrl+r forces an immediate refresh when you need one.
const DescribeCacheTTL = 24 * time.Hour

type sobjectCacheEnvelope struct {
	CachedAt time.Time        `json:"cachedAt"`
	Org      string           `json:"org"`
	SObjects []SObjectSummary `json:"sobjects"`
}

type describeCacheEnvelope struct {
	CachedAt time.Time `json:"cachedAt"`
	Org      string    `json:"org"`
	Describe Describe  `json:"describe"`
}

// cacheRoot is <configDir>/cache, created on demand. configDir() lives in
// storage.go and already resolves the per-OS config location.
func cacheRoot() (string, error) {
	d, err := configDir()
	if err != nil {
		return "", err
	}
	root := filepath.Join(d, "cache")
	if err := os.MkdirAll(root, 0o755); err != nil {
		return "", err
	}
	return root, nil
}

func orgCacheDir(org string) (string, error) {
	root, err := cacheRoot()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(root, orgDirName(org))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	return dir, nil
}

// orgDirName builds a stable, filesystem-safe directory name for an org. It pairs
// a sanitized form (readable when poking around the cache) with a short hash of
// the raw value, so distinct orgs that sanitize to the same string (e.g. "a@b"
// and "a_b") never collide.
func orgDirName(org string) string {
	h := fnv.New32a()
	_, _ = h.Write([]byte(org))
	return sanitizeFileName(org) + "-" + fmt.Sprintf("%08x", h.Sum32())
}

func sanitizeFileName(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9',
			r == '.', r == '_', r == '-':
			b.WriteRune(r)
		default:
			b.WriteRune('_')
		}
	}
	out := b.String()
	if out == "" {
		return "org"
	}
	return out
}

// writeJSONAtomic writes via a temp file + rename so a crash mid-write can never
// leave a half-written cache file that fails to parse later.
func writeJSONAtomic(path string, v any) error {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func cacheFresh(t time.Time) bool {
	return !t.IsZero() && time.Since(t) < DescribeCacheTTL
}

// --- sobject list ---

func readSObjectCache(org string) ([]SObjectSummary, time.Time, bool) {
	dir, err := orgCacheDir(org)
	if err != nil {
		return nil, time.Time{}, false
	}
	b, err := os.ReadFile(filepath.Join(dir, "sobjects.json"))
	if err != nil {
		return nil, time.Time{}, false
	}
	var env sobjectCacheEnvelope
	if err := json.Unmarshal(b, &env); err != nil || len(env.SObjects) == 0 {
		return nil, time.Time{}, false
	}
	return env.SObjects, env.CachedAt, true
}

func writeSObjectCache(org string, ss []SObjectSummary) {
	dir, err := orgCacheDir(org)
	if err != nil {
		return // best-effort: a cache write failure must never break a load
	}
	_ = writeJSONAtomic(filepath.Join(dir, "sobjects.json"), sobjectCacheEnvelope{
		CachedAt: time.Now(),
		Org:      org,
		SObjects: ss,
	})
}

// --- describe ---

func describeCachePath(org, sobject string) (string, error) {
	dir, err := orgCacheDir(org)
	if err != nil {
		return "", err
	}
	ddir := filepath.Join(dir, "describe")
	if err := os.MkdirAll(ddir, 0o755); err != nil {
		return "", err
	}
	return filepath.Join(ddir, lc(sobject)+".json"), nil
}

func readDescribeCache(org, sobject string) (Describe, time.Time, bool) {
	path, err := describeCachePath(org, sobject)
	if err != nil {
		return Describe{}, time.Time{}, false
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return Describe{}, time.Time{}, false
	}
	var env describeCacheEnvelope
	if err := json.Unmarshal(b, &env); err != nil || env.Describe.Name == "" {
		return Describe{}, time.Time{}, false
	}
	return env.Describe, env.CachedAt, true
}

func writeDescribeCache(org string, d Describe) {
	if d.Name == "" {
		return
	}
	path, err := describeCachePath(org, d.Name)
	if err != nil {
		return
	}
	_ = writeJSONAtomic(path, describeCacheEnvelope{
		CachedAt: time.Now(),
		Org:      org,
		Describe: d,
	})
}

// HumanAge renders a cache age compactly for the status line ("just now", "5m
// ago", "2h ago", "3d ago").
func HumanAge(t time.Time) string {
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd ago", int(d.Hours())/24)
	}
}
