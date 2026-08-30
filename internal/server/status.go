package server

import (
	"fmt"
	"slices"
	"time"
)

// Status is a server's lifecycle state.
//
//	Stopped -> Starting -> Running -> Stopping -> Stopped
//	                 \                    ^
//	                  \-> Unknown (session gone while we believed it was up)
//
// Unknown exists to prevent a double launch, so it refuses Start until the
// operator marks the server Stopped.
type Status uint8

const (
	StatusStopped Status = iota
	StatusStarting
	StatusRunning
	StatusStopping
	StatusUnknown
)

var statusNames = [...]string{
	StatusStopped:  "stopped",
	StatusStarting: "starting",
	StatusRunning:  "running",
	StatusStopping: "stopping",
	StatusUnknown:  "unknown",
}

// statusEdges omits the self-edge every status has; CanTransitionTo adds it.
var statusEdges = map[Status][]Status{
	StatusStopped:  {StatusStarting},
	StatusStarting: {StatusRunning, StatusStopped, StatusUnknown},
	StatusRunning:  {StatusStopping, StatusUnknown},
	StatusStopping: {StatusStopped, StatusUnknown},
	StatusUnknown:  {StatusStopped},
}

func ParseStatus(s string) (Status, error) {
	for i, name := range statusNames {
		if name == s {
			return Status(i), nil
		}
	}
	return 0, fmt.Errorf("status %q: not one of %v", s, statusNames)
}

func (s Status) String() string {
	if int(s) >= len(statusNames) {
		return fmt.Sprintf("status(%d)", uint8(s))
	}
	return statusNames[s]
}

func (s Status) MarshalText() ([]byte, error) {
	if int(s) >= len(statusNames) {
		return nil, fmt.Errorf("status(%d): out of range", uint8(s))
	}
	return []byte(statusNames[s]), nil
}

func (s *Status) UnmarshalText(b []byte) error {
	parsed, err := ParseStatus(string(b))
	if err != nil {
		return err
	}
	*s = parsed
	return nil
}

// CanTransitionTo reports whether next is reachable from s. A status may always
// transition to itself, so a repeated reconcile is a no-op.
func (s Status) CanTransitionTo(next Status) bool {
	return s == next || slices.Contains(statusEdges[s], next)
}

// CanStart reports whether a Start is legal. Only Stopped is.
func (s Status) CanStart() bool { return s == StatusStopped }

// ForceKillAllowed is derived, never stored: a stop that has outlived its
// timeout. It is not a Status value.
func ForceKillAllowed(s Status, stoppingFor, timeout time.Duration) bool {
	return s == StatusStopping && stoppingFor >= timeout
}
