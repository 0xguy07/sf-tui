package sf

import (
	"sync"

	tea "github.com/charmbracelet/bubbletea"
)

// Cache holds per-org SObject metadata for autocomplete.
type Cache struct {
	mu        sync.RWMutex
	sobjects  map[string][]SObjectSummary // org -> list
	describes map[string]map[string]Describe // org -> sobject name (lowercased) -> describe
	loading   map[string]bool               // org+sobject key -> in-flight
}

func NewCache() *Cache {
	return &Cache{
		sobjects:  map[string][]SObjectSummary{},
		describes: map[string]map[string]Describe{},
		loading:   map[string]bool{},
	}
}

func (c *Cache) SetSObjects(org string, ss []SObjectSummary) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.sobjects[org] = ss
}

func (c *Cache) SObjects(org string) []SObjectSummary {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.sobjects[org]
}

func (c *Cache) HasSObjects(org string) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	_, ok := c.sobjects[org]
	return ok
}

func (c *Cache) SetDescribe(org string, d Describe) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.describes[org] == nil {
		c.describes[org] = map[string]Describe{}
	}
	c.describes[org][lc(d.Name)] = d
}

func (c *Cache) Describe(org, sobject string) (Describe, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if m, ok := c.describes[org]; ok {
		d, ok := m[lc(sobject)]
		return d, ok
	}
	return Describe{}, false
}

// TryClaimDescribe returns true if the caller should launch the describe command.
// Subsequent callers for the same org+sobject get false until ReleaseDescribe is called
// or the describe is stored.
func (c *Cache) TryClaimDescribe(org, sobject string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	key := org + "\x00" + lc(sobject)
	if c.loading[key] {
		return false
	}
	if m, ok := c.describes[org]; ok {
		if _, ok := m[lc(sobject)]; ok {
			return false
		}
	}
	c.loading[key] = true
	return true
}

func (c *Cache) ReleaseDescribe(org, sobject string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.loading, org+"\x00"+lc(sobject))
}

// EnsureDescribe returns the describe if cached, or a tea.Cmd that will load it.
// If another load is already in flight, cmd is nil.
func (c *Cache) EnsureDescribe(org, sobject string) (Describe, bool, tea.Cmd) {
	if d, ok := c.Describe(org, sobject); ok {
		return d, true, nil
	}
	if !c.TryClaimDescribe(org, sobject) {
		return Describe{}, false, nil
	}
	return Describe{}, false, LoadDescribe(org, sobject)
}

func lc(s string) string {
	b := make([]byte, len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= 'A' && c <= 'Z' {
			c += 'a' - 'A'
		}
		b[i] = c
	}
	return string(b)
}
