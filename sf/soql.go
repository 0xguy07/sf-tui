package sf

import (
	"encoding/json"
	"fmt"
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
