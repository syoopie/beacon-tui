package config

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/BurntSushi/toml"

	"github.com/syoopie/beacon-tui/internal/server"
)

// LoadSpecs reads every servers/*.toml, sorted by ID. A missing servers dir is
// not an error; it returns nil.
func LoadSpecs(dirs Dirs) ([]server.Spec, error) {
	dir := dirs.ServersDir()

	entries, err := os.ReadDir(dir)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", dir, err)
	}

	var specs []server.Spec
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".toml") {
			continue
		}
		path := filepath.Join(dir, e.Name())
		s, err := LoadSpec(path)
		if err != nil {
			return nil, err
		}
		if want := strings.TrimSuffix(e.Name(), ".toml"); s.ID.String() != want {
			return nil, fmt.Errorf("%s: id %q does not match the file name", path, s.ID)
		}
		specs = append(specs, s)
	}

	slices.SortFunc(specs, func(a, b server.Spec) int { return strings.Compare(a.ID.String(), b.ID.String()) })
	return specs, nil
}

// LoadSpec reads one file.
func LoadSpec(path string) (server.Spec, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return server.Spec{}, fmt.Errorf("reading %s: %w", path, err)
	}

	var s server.Spec
	md, err := toml.Decode(string(data), &s)
	if err != nil {
		return server.Spec{}, fmt.Errorf("%s: %w", path, err)
	}
	if undecoded := md.Undecoded(); len(undecoded) > 0 {
		return server.Spec{}, fmt.Errorf("%s: unknown key %q", path, undecoded[0].String())
	}
	if err := s.Validate(); err != nil {
		return server.Spec{}, fmt.Errorf("%s: %w", path, err)
	}
	return s, nil
}

// SaveSpec writes servers/<id>.toml at 0600 because the file can hold an RCON password.
func SaveSpec(dirs Dirs, s server.Spec) error {
	path := dirs.ServerFile(s.ID)
	if err := s.Validate(); err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}

	var buf bytes.Buffer
	if err := toml.NewEncoder(&buf).Encode(s); err != nil {
		return fmt.Errorf("encoding %s: %w", path, err)
	}
	return WriteFileAtomic(path, buf.Bytes(), 0o600)
}
