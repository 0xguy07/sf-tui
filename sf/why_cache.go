package sf

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

// The Why tab caches schema and automation metadata only. History values and
// record data are never written to disk.

type globalDescribeEnvelope struct {
	CachedAt time.Time        `json:"cachedAt"`
	Org      string           `json:"org"`
	SObjects []SObjectSummary `json:"sobjects"`
}

type automationCacheEnvelope struct {
	CachedAt time.Time           `json:"cachedAt"`
	Org      string              `json:"org"`
	Object   string              `json:"object"`
	Sections []AutomationSection `json:"sections"`
}

func orgSubdir(org, sub string) (string, error) {
	dir, err := orgCacheDir(org)
	if err != nil {
		return "", err
	}
	d := filepath.Join(dir, sub)
	if err := os.MkdirAll(d, 0o755); err != nil {
		return "", err
	}
	return d, nil
}

func readJSONFile(path string, v any) bool {
	b, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	return json.Unmarshal(b, v) == nil
}

func readGlobalDescribeCache(org string) ([]SObjectSummary, time.Time, bool) {
	dir, err := orgCacheDir(org)
	if err != nil {
		return nil, time.Time{}, false
	}
	var env globalDescribeEnvelope
	if !readJSONFile(filepath.Join(dir, "globaldescribe.json"), &env) || len(env.SObjects) == 0 {
		return nil, time.Time{}, false
	}
	return env.SObjects, env.CachedAt, true
}

func writeGlobalDescribeCache(org string, ss []SObjectSummary) {
	dir, err := orgCacheDir(org)
	if err != nil || len(ss) == 0 {
		return
	}
	_ = writeJSONAtomic(filepath.Join(dir, "globaldescribe.json"), globalDescribeEnvelope{CachedAt: time.Now(), Org: org, SObjects: ss})
}

func readAutomationCache(org, obj string) (AutomationPart, bool) {
	dir, err := orgSubdir(org, "why")
	if err != nil {
		return AutomationPart{}, false
	}
	var env automationCacheEnvelope
	if !readJSONFile(filepath.Join(dir, lc(obj)+".json"), &env) || !cacheFresh(env.CachedAt) || len(env.Sections) == 0 {
		return AutomationPart{}, false
	}
	return AutomationPart{Sections: env.Sections}, true
}

func writeAutomationCache(org, obj string, p AutomationPart) {
	dir, err := orgSubdir(org, "why")
	if err != nil {
		return
	}
	_ = writeJSONAtomic(filepath.Join(dir, lc(obj)+".json"), automationCacheEnvelope{CachedAt: time.Now(), Org: org, Object: obj, Sections: p.Sections})
}

func readFlowMetaCache(org, versionID string) (json.RawMessage, bool) {
	dir, err := orgSubdir(org, "flowmeta")
	if err != nil {
		return nil, false
	}
	b, err := os.ReadFile(filepath.Join(dir, sanitizeFileName(versionID)+".json"))
	if err != nil || !json.Valid(b) {
		return nil, false
	}
	return b, true
}

func writeFlowMetaCache(org, versionID string, raw json.RawMessage) {
	dir, err := orgSubdir(org, "flowmeta")
	if err != nil {
		return
	}
	_ = writeJSONAtomic(filepath.Join(dir, sanitizeFileName(versionID)+".json"), raw)
}
