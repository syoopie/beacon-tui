package server

import (
	"errors"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
)

// maxIDLen bounds an ID so it stays a usable file-name component.
const maxIDLen = 64

// ID is a server identifier derived from a directory name. Branded so it cannot
// be confused with a path or a session name.
type ID string

// ParseID rejects anything that is not a safe file-name component:
// lowercase letters, digits, '-' and '_', 1..64 chars, not "." or "..".
func ParseID(s string) (ID, error) {
	if s == "" {
		return "", errors.New("server id: empty")
	}
	if len(s) > maxIDLen {
		return "", fmt.Errorf("server id %q: longer than %d characters", s, maxIDLen)
	}
	if s == "." || s == ".." {
		return "", fmt.Errorf("server id %q: reserved name", s)
	}
	for _, r := range s {
		if !idRune(r) {
			return "", fmt.Errorf("server id %q: character %q is not lowercase letter, digit, '-' or '_'", s, r)
		}
	}
	return ID(s), nil
}

func idRune(r rune) bool {
	switch {
	case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '-', r == '_':
		return true
	}
	return false
}

// IDFromDir derives a base ID from a directory path: the base name lowercased,
// with every run of unsupported characters collapsed to '-', trimmed of leading
// and trailing '-'. Returns an error when nothing usable survives.
func IDFromDir(dir string) (ID, error) {
	base := filepath.Base(filepath.Clean(dir))

	var b strings.Builder
	dashed := false
	for _, r := range strings.ToLower(base) {
		if idRune(r) {
			b.WriteRune(r)
			dashed = false
			continue
		}
		if !dashed {
			b.WriteByte('-')
			dashed = true
		}
	}

	s := strings.Trim(b.String(), "-")
	if len(s) > maxIDLen {
		s = strings.TrimRight(s[:maxIDLen], "-")
	}
	if s == "" || s == "." || s == ".." {
		return "", fmt.Errorf("directory %q: no usable server id", dir)
	}
	return ParseID(s)
}

// NextFreeID appends -2, -3, ... until the result is unused.
func NextFreeID(base ID, taken map[ID]bool) ID {
	if !taken[base] {
		return base
	}
	for n := 2; ; n++ {
		suffix := "-" + strconv.Itoa(n)
		stem := string(base)
		if len(stem)+len(suffix) > maxIDLen {
			stem = strings.TrimRight(stem[:maxIDLen-len(suffix)], "-")
		}
		candidate := ID(stem + suffix)
		if !taken[candidate] {
			return candidate
		}
	}
}

func (id ID) String() string { return string(id) }

func (id ID) MarshalText() ([]byte, error) { return []byte(id), nil }

func (id *ID) UnmarshalText(b []byte) error {
	parsed, err := ParseID(string(b))
	if err != nil {
		return err
	}
	*id = parsed
	return nil
}
