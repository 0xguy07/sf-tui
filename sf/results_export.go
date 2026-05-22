package sf

import (
	"encoding/csv"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// FormatTSV serializes columns + rows as TSV (header row + data rows).
// Tabs and newlines inside values are replaced with spaces so each row stays a
// single line — pasting into Sheets stays clean.
func FormatTSV(cols []string, rows []QueryRecord) string {
	var b strings.Builder
	for i, c := range cols {
		if i > 0 {
			b.WriteByte('\t')
		}
		b.WriteString(c)
	}
	b.WriteByte('\n')
	for _, rec := range rows {
		for i, c := range cols {
			if i > 0 {
				b.WriteByte('\t')
			}
			b.WriteString(sanitizeTSV(stringifyValue(ResolvePath(rec, c))))
		}
		b.WriteByte('\n')
	}
	return b.String()
}

func sanitizeTSV(s string) string {
	s = strings.ReplaceAll(s, "\t", " ")
	s = strings.ReplaceAll(s, "\r", " ")
	s = strings.ReplaceAll(s, "\n", " ")
	return s
}

// stringifyValue renders a JSON-decoded SOQL value as a flat string suitable
// for export. Mirrors the formatting in panes/results.go but without width
// clipping.
func stringifyValue(v any) string {
	if v == nil {
		return ""
	}
	switch x := v.(type) {
	case string:
		return x
	case bool:
		if x {
			return "true"
		}
		return "false"
	case float64:
		if x == float64(int64(x)) {
			return fmt.Sprintf("%d", int64(x))
		}
		return fmt.Sprintf("%g", x)
	default:
		return fmt.Sprintf("%v", v)
	}
}

// ExportCSV writes columns + rows as CSV to ~/sf-tui-queries/<name>.csv.
// If name is empty, uses a timestamp.
func ExportCSV(name string, cols []string, rows []QueryRecord) (string, error) {
	dir, err := ExportDir()
	if err != nil {
		return "", err
	}
	base := strings.TrimSpace(name)
	if base == "" {
		base = "results-" + time.Now().Format("2006-01-02-150405")
	}
	base = unsafeFilename.ReplaceAllString(base, "-")
	if !strings.HasSuffix(strings.ToLower(base), ".csv") {
		base += ".csv"
	}
	path := filepath.Join(dir, base)

	f, err := os.Create(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	w := csv.NewWriter(f)
	if err := w.Write(cols); err != nil {
		return "", err
	}
	for _, rec := range rows {
		row := make([]string, len(cols))
		for i, c := range cols {
			row[i] = stringifyValue(ResolvePath(rec, c))
		}
		if err := w.Write(row); err != nil {
			return "", err
		}
	}
	w.Flush()
	if err := w.Error(); err != nil {
		return "", err
	}
	return path, nil
}
