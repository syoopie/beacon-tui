package server

import (
	"testing"
	"time"
)

func TestStatusExhaustive(t *testing.T) {
	for i := range statusNames {
		s := Status(i)
		name := statusNames[i]
		if name == "" {
			t.Errorf("Status(%d) has no wire name", i)
			continue
		}
		if got := s.String(); got != name {
			t.Errorf("Status(%d).String() = %q, want %q", i, got, name)
		}
		parsed, err := ParseStatus(name)
		if err != nil {
			t.Errorf("ParseStatus(%q): %v", name, err)
			continue
		}
		if parsed != s {
			t.Errorf("ParseStatus(%q) = %v, want %v", name, parsed, s)
		}
		if _, ok := statusEdges[s]; !ok {
			t.Errorf("Status %q missing from statusEdges", name)
		}
		text, err := s.MarshalText()
		if err != nil {
			t.Errorf("Status(%d).MarshalText(): %v", i, err)
			continue
		}
		var back Status
		if err := back.UnmarshalText(text); err != nil {
			t.Errorf("Status.UnmarshalText(%q): %v", text, err)
			continue
		}
		if back != s {
			t.Errorf("round trip of %v yielded %v", s, back)
		}
	}
	if len(statusEdges) != len(statusNames) {
		t.Errorf("statusEdges has %d entries, statusNames has %d", len(statusEdges), len(statusNames))
	}
}

func TestParseStatusRejects(t *testing.T) {
	for _, in := range []string{"halted", "", "Stopped", "running "} {
		if got, err := ParseStatus(in); err == nil {
			t.Errorf("ParseStatus(%q) = %v, want error", in, got)
		}
	}
}

func TestStatusCanTransitionTo(t *testing.T) {
	tests := []struct {
		from, to Status
		want     bool
	}{
		{StatusStopped, StatusStopped, true},
		{StatusStopped, StatusStarting, true},
		{StatusStopped, StatusRunning, false},
		{StatusStopped, StatusUnknown, false},
		{StatusStarting, StatusStarting, true},
		{StatusStarting, StatusRunning, true},
		{StatusStarting, StatusStopped, true},
		{StatusStarting, StatusUnknown, true},
		{StatusStarting, StatusStopping, false},
		{StatusRunning, StatusRunning, true},
		{StatusRunning, StatusStopping, true},
		{StatusRunning, StatusUnknown, true},
		{StatusRunning, StatusStopped, false},
		{StatusRunning, StatusStarting, false},
		{StatusStopping, StatusStopping, true},
		{StatusStopping, StatusStopped, true},
		{StatusStopping, StatusUnknown, true},
		{StatusStopping, StatusRunning, false},
		{StatusUnknown, StatusUnknown, true},
		{StatusUnknown, StatusStopped, true},
		{StatusUnknown, StatusStarting, false},
		{StatusUnknown, StatusRunning, false},
	}
	for _, tt := range tests {
		if got := tt.from.CanTransitionTo(tt.to); got != tt.want {
			t.Errorf("%v.CanTransitionTo(%v) = %v, want %v", tt.from, tt.to, got, tt.want)
		}
	}
}

func TestStatusCanStart(t *testing.T) {
	for i := range statusNames {
		s := Status(i)
		want := s == StatusStopped
		if got := s.CanStart(); got != want {
			t.Errorf("%v.CanStart() = %v, want %v", s, got, want)
		}
	}
}

func TestForceKillAllowed(t *testing.T) {
	const timeout = 60 * time.Second
	tests := []struct {
		status      Status
		stoppingFor time.Duration
		want        bool
	}{
		{StatusStopping, 90 * time.Second, true},
		{StatusStopping, timeout, true},
		{StatusStopping, 59 * time.Second, false},
		{StatusRunning, 90 * time.Second, false},
		{StatusStarting, 90 * time.Second, false},
		{StatusStopped, 90 * time.Second, false},
		{StatusUnknown, 90 * time.Second, false},
	}
	for _, tt := range tests {
		if got := ForceKillAllowed(tt.status, tt.stoppingFor, timeout); got != tt.want {
			t.Errorf("ForceKillAllowed(%v, %v, %v) = %v, want %v", tt.status, tt.stoppingFor, timeout, got, tt.want)
		}
	}
}
