package sf

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os/exec"
)

// SOQLRunner runs a SOQL query against an org and returns the raw
// `sf data query --json` output. It is the only I/O seam the permissions
// builders use, so tests can substitute fixture files.
type SOQLRunner interface {
	SOQL(org, soql string, tooling bool) ([]byte, error)
}

type sfRunner struct{}

func (sfRunner) SOQL(org, soql string, tooling bool) ([]byte, error) {
	return runSOQLRaw(org, soql, tooling)
}

// DefaultSOQLRunner returns the runner that shells out to the sf CLI.
func DefaultSOQLRunner() SOQLRunner { return sfRunner{} }

// decodeRecords extracts result.records from `sf data query --json` output.
func decodeRecords(out []byte) ([]map[string]any, error) {
	var r struct {
		Result struct {
			Records []map[string]any `json:"records"`
		} `json:"result"`
	}
	if err := json.Unmarshal(out, &r); err != nil {
		return nil, fmt.Errorf("parse query result: %w", err)
	}
	return r.Result.Records, nil
}

// sfCommandError turns a failed `sf … --json` run into a useful error. Since
// sf CLI v48, --json writes the error object to stdout; stderr only carries
// warnings. So prefer the stdout JSON message, then stderr, then the exit error.
func sfCommandError(out []byte, err error) error {
	var r struct {
		Name    string `json:"name"`
		Message string `json:"message"`
	}
	if jerr := json.Unmarshal(out, &r); jerr == nil && r.Message != "" {
		if r.Name != "" {
			return fmt.Errorf("%s: %s", r.Name, r.Message)
		}
		return fmt.Errorf("%s", r.Message)
	}
	if ee, ok := err.(*exec.ExitError); ok && len(bytes.TrimSpace(ee.Stderr)) > 0 {
		return fmt.Errorf("%s", bytes.TrimSpace(ee.Stderr))
	}
	return fmt.Errorf("sf: %w", err)
}
