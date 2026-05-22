package sf

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type SavedQuery struct {
	Name    string    `json:"name,omitempty"`
	Org     string    `json:"org"`
	SOQL    string    `json:"soql"`
	RanAt   time.Time `json:"ranAt,omitempty"`
	Saved   bool      `json:"saved"`
}

type Store struct {
	Queries []SavedQuery `json:"queries"`
}

const maxHistory = 100

func configDir() (string, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(base, "sf-tui")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	return dir, nil
}

func storePath() (string, error) {
	d, err := configDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(d, "queries.json"), nil
}

func LoadStore() (*Store, error) {
	p, err := storePath()
	if err != nil {
		return nil, err
	}
	b, err := os.ReadFile(p)
	if err != nil {
		if os.IsNotExist(err) {
			return &Store{}, nil
		}
		return nil, err
	}
	var s Store
	if err := json.Unmarshal(b, &s); err != nil {
		return &Store{}, nil
	}
	return &s, nil
}

func (s *Store) Save() error {
	p, err := storePath()
	if err != nil {
		return err
	}
	b, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(p, b, 0o644)
}

func (s *Store) AddHistory(org, soql string) {
	soql = strings.TrimSpace(soql)
	if soql == "" {
		return
	}
	// dedupe: if most recent history entry for this org matches, bump its time instead
	for i, q := range s.Queries {
		if !q.Saved && q.Org == org && q.SOQL == soql {
			s.Queries[i].RanAt = time.Now()
			return
		}
	}
	s.Queries = append(s.Queries, SavedQuery{
		Org:   org,
		SOQL:  soql,
		RanAt: time.Now(),
	})
	s.trimHistory()
}

func (s *Store) trimHistory() {
	hist := []SavedQuery{}
	saved := []SavedQuery{}
	for _, q := range s.Queries {
		if q.Saved {
			saved = append(saved, q)
		} else {
			hist = append(hist, q)
		}
	}
	sort.Slice(hist, func(i, j int) bool { return hist[i].RanAt.After(hist[j].RanAt) })
	if len(hist) > maxHistory {
		hist = hist[:maxHistory]
	}
	s.Queries = append(saved, hist...)
}

func (s *Store) SaveNamed(name, org, soql string) {
	soql = strings.TrimSpace(soql)
	if soql == "" || name == "" {
		return
	}
	for i, q := range s.Queries {
		if q.Saved && q.Name == name {
			s.Queries[i].Org = org
			s.Queries[i].SOQL = soql
			return
		}
	}
	s.Queries = append(s.Queries, SavedQuery{
		Name:  name,
		Org:   org,
		SOQL:  soql,
		Saved: true,
	})
}

// Entries returns saved queries first (alphabetical), then history newest first.
func (s *Store) Entries() []SavedQuery {
	saved := []SavedQuery{}
	hist := []SavedQuery{}
	for _, q := range s.Queries {
		if q.Saved {
			saved = append(saved, q)
		} else {
			hist = append(hist, q)
		}
	}
	sort.Slice(saved, func(i, j int) bool { return saved[i].Name < saved[j].Name })
	sort.Slice(hist, func(i, j int) bool { return hist[i].RanAt.After(hist[j].RanAt) })
	return append(saved, hist...)
}
