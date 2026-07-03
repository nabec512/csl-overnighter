// Package profile handles loading, saving, and listing parking-permit
// profiles persisted as individual JSON files on disk.
package profile

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Profile holds everything needed to fill in and submit the overnight
// parking permit form for one vehicle/household.
//
// Fields is intentionally generic (key -> value) because the exact set of
// form fields on the town's site is not finalized yet. Once the real form
// is known, promote the fields it needs to named struct fields and keep
// Fields only for anything left over.
type Profile struct {
	Name      string            `json:"name"`
	Fields    map[string]string `json:"fields"`
	CreatedAt time.Time         `json:"created_at"`
	UpdatedAt time.Time         `json:"updated_at"`
}

// Store reads and writes profiles as JSON files in a directory, one file
// per profile, named "<profile-name>.json".
type Store struct {
	Dir string
}

// DefaultDir returns the standard location for profile storage:
// $XDG_CONFIG_HOME/csl-overnighter/profiles (or the OS equivalent).
func DefaultDir() (string, error) {
	cfgDir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("resolve user config dir: %w", err)
	}
	return filepath.Join(cfgDir, "csl-overnighter", "profiles"), nil
}

// NewStore creates a Store rooted at dir, creating the directory if needed.
func NewStore(dir string) (*Store, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("create profile dir %s: %w", dir, err)
	}
	return &Store{Dir: dir}, nil
}

func (s *Store) path(name string) string {
	return filepath.Join(s.Dir, name+".json")
}

// Save writes p to disk, creating or overwriting its file.
func (s *Store) Save(p *Profile) error {
	if p.Name == "" {
		return fmt.Errorf("profile name must not be empty")
	}
	now := time.Now()
	if p.CreatedAt.IsZero() {
		p.CreatedAt = now
	}
	p.UpdatedAt = now

	data, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal profile %s: %w", p.Name, err)
	}
	if err := os.WriteFile(s.path(p.Name), data, 0o600); err != nil {
		return fmt.Errorf("write profile %s: %w", p.Name, err)
	}
	return nil
}

// Load reads the named profile from disk.
func (s *Store) Load(name string) (*Profile, error) {
	data, err := os.ReadFile(s.path(name))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("profile %q not found in %s", name, s.Dir)
		}
		return nil, fmt.Errorf("read profile %s: %w", name, err)
	}
	var p Profile
	if err := json.Unmarshal(data, &p); err != nil {
		return nil, fmt.Errorf("parse profile %s: %w", name, err)
	}
	return &p, nil
}

// Delete removes the named profile's file.
func (s *Store) Delete(name string) error {
	if err := os.Remove(s.path(name)); err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("profile %q not found in %s", name, s.Dir)
		}
		return fmt.Errorf("delete profile %s: %w", name, err)
	}
	return nil
}

// List returns the names of all saved profiles, sorted alphabetically.
func (s *Store) List() ([]string, error) {
	entries, err := os.ReadDir(s.Dir)
	if err != nil {
		return nil, fmt.Errorf("read profile dir %s: %w", s.Dir, err)
	}
	var names []string
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		names = append(names, strings.TrimSuffix(e.Name(), ".json"))
	}
	sort.Strings(names)
	return names, nil
}

// ParseFieldFlags converts repeated "key=value" strings (as passed via a
// repeatable --field flag) into a map.
func ParseFieldFlags(raw []string) (map[string]string, error) {
	fields := make(map[string]string, len(raw))
	for _, kv := range raw {
		key, value, ok := strings.Cut(kv, "=")
		if !ok || key == "" {
			return nil, fmt.Errorf("invalid --field %q, expected key=value", kv)
		}
		fields[key] = value
	}
	return fields, nil
}
