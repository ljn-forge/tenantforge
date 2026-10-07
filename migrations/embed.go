package migrations

import (
	"embed"
	"errors"
	"io/fs"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
)

//go:embed *.sql
var embedded embed.FS

// Sources combines embedded SQL migration roots without mutable global Goose state.
type Sources struct {
	mu       sync.RWMutex
	files    map[string]fs.FS
	entries  map[string]fs.DirEntry
	versions map[int]string
	frozen   bool
}

func NewSources() *Sources {
	return &Sources{files: map[string]fs.FS{}, entries: map[string]fs.DirEntry{}, versions: map[int]string{}}
}

var FS = func() *Sources {
	s := NewSources()
	if err := s.Register(embedded); err != nil {
		panic("invalid built-in migrations")
	}
	return s
}()
var migrationName = regexp.MustCompile(`^[0-9]+_[a-z0-9_]+\.sql$`)

// RegisterFS adds a versioned SQL migration root before serve/migrate startup.
func RegisterFS(source fs.FS) error { return FS.Register(source) }
func (s *Sources) Register(source fs.FS) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.frozen {
		return errors.New("migration sources frozen")
	}
	entries, e := fs.ReadDir(source, ".")
	if e != nil {
		return errors.New("migration root unavailable")
	}
	pending := map[int]string{}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".sql") {
			continue
		}
		name := entry.Name()
		if !migrationName.MatchString(name) {
			return errors.New("invalid migration filename")
		}
		version, e := strconv.Atoi(strings.SplitN(name, "_", 2)[0])
		if e != nil || version < 1 {
			return errors.New("invalid migration version")
		}
		if _, ok := s.versions[version]; ok {
			return errors.New("duplicate migration version")
		}
		if _, ok := pending[version]; ok {
			return errors.New("duplicate migration version")
		}
		pending[version] = name
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".sql") {
			continue
		}
		name := entry.Name()
		version, _ := strconv.Atoi(strings.SplitN(name, "_", 2)[0])
		s.versions[version] = name
		s.entries[name] = entry
		s.files[name] = source
	}
	return nil
}
func (s *Sources) Open(name string) (fs.File, error) {
	s.mu.RLock()
	source := s.files[name]
	s.mu.RUnlock()
	if source == nil {
		return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrNotExist}
	}
	return source.Open(name)
}
func (s *Sources) ReadDir(name string) ([]fs.DirEntry, error) {
	if name != "." {
		return nil, fs.ErrNotExist
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := []fs.DirEntry{}
	for _, entry := range s.entries {
		out = append(out, entry)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name() < out[j].Name() })
	return out, nil
}
func (s *Sources) Freeze() { s.mu.Lock(); defer s.mu.Unlock(); s.frozen = true }
func LatestVersion() int {
	FS.mu.RLock()
	defer FS.mu.RUnlock()
	latest := 0
	for version := range FS.versions {
		if version > latest {
			latest = version
		}
	}
	return latest
}

// Versions lists every registered migration, including extensions whose version numbers exceed future core upgrades.
func Versions() []int {
	FS.mu.RLock()
	defer FS.mu.RUnlock()
	out := make([]int, 0, len(FS.versions))
	for version := range FS.versions {
		out = append(out, version)
	}
	sort.Ints(out)
	return out
}
