package server

import (
	"fmt"
	"strings"
	"unicode"
)

// SessionPrefix keeps beacon's tmux sessions distinguishable from the operator's own.
const SessionPrefix = "beacon-"

// Session is a tmux session name.
type Session string

func SessionFor(id ID) Session { return Session(SessionPrefix + string(id)) }

func (s Session) String() string { return string(s) }

func (s Session) MarshalText() ([]byte, error) { return []byte(s), nil }

func (s *Session) UnmarshalText(b []byte) error {
	parsed := Session(b)
	if err := parsed.validate(); err != nil {
		return err
	}
	*s = parsed
	return nil
}

func (s Session) validate() error {
	rest, ok := strings.CutPrefix(string(s), SessionPrefix)
	if !ok {
		return fmt.Errorf("tmux session %q: must start with %q", s, SessionPrefix)
	}
	if rest == "" {
		return fmt.Errorf("tmux session %q: nothing after the %q prefix", s, SessionPrefix)
	}
	for _, r := range s {
		if unicode.IsSpace(r) {
			return fmt.Errorf("tmux session %q: contains whitespace", s)
		}
	}
	return nil
}
