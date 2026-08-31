package server

import (
	"strings"
	"testing"
)

func validSpec() Spec {
	return Spec{
		ID:      "survival",
		Dir:     "/srv/minecraft/survival",
		Start:   "./run.sh",
		Script:  "run.sh",
		Port:    25565,
		Session: SessionFor("survival"),
		LogFile: "/srv/minecraft/survival/logs/beacon.log",
		Exec:    ExecOK,
		RCON:    RCON{Enabled: true, Port: 25575, Password: "hunter2"},
		State:   State{LastKnown: StatusStopped},
	}
}

func TestSpecValidate(t *testing.T) {
	if err := validSpec().Validate(); err != nil {
		t.Fatalf("valid spec rejected: %v", err)
	}

	tests := []struct {
		name   string
		mutate func(*Spec)
		field  string
	}{
		{"empty id", func(s *Spec) { s.ID = "" }, "id"},
		{"bad id", func(s *Spec) { s.ID = "Survival World" }, "id"},
		{"empty dir", func(s *Spec) { s.Dir = "" }, "dir"},
		{"relative dir", func(s *Spec) { s.Dir = "srv/minecraft/survival" }, "dir"},
		{"empty start", func(s *Spec) { s.Start = "" }, "start"},
		{"zero port", func(s *Spec) { s.Port = 0 }, "port"},
		{"negative port", func(s *Spec) { s.Port = -1 }, "port"},
		{"port too high", func(s *Spec) { s.Port = 65536 }, "port"},
		{"empty session", func(s *Spec) { s.Session = "" }, "session"},
		{"unprefixed session", func(s *Spec) { s.Session = "survival" }, "session"},
		{"empty log file", func(s *Spec) { s.LogFile = "" }, "log_file"},
		{"relative log file", func(s *Spec) { s.LogFile = "logs/beacon.log" }, "log_file"},
		{"rcon port too high", func(s *Spec) { s.RCON.Port = 70000 }, "rcon.port"},
		{"negative rcon port", func(s *Spec) { s.RCON.Port = -1 }, "rcon.port"},
		{"negative pid", func(s *Spec) { s.State.PID = -1 }, "state.pid"},
		{"bad completion", func(s *Spec) { s.Commands.Completion = "sometimes" }, "completion"},
		{"bad mc_version", func(s *Spec) { s.Commands.MCVersion = "1.x" }, "mc_version"},
		{"four-part mc_version", func(s *Spec) { s.Commands.MCVersion = "1.20.1.2" }, "mc_version"},
		{"unknown loader", func(s *Spec) { s.Commands.Loader = "frobnix" }, "loader"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := validSpec()
			tt.mutate(&s)
			err := s.Validate()
			if err == nil {
				t.Fatalf("Validate accepted %s", tt.name)
			}
			if !strings.Contains(err.Error(), tt.field) {
				t.Fatalf("Validate error %q does not name %q", err, tt.field)
			}
		})
	}
}

func TestSpecValidateAllowsEmptyScriptAndRCONPort(t *testing.T) {
	s := validSpec()
	s.Script = ""
	s.RCON = RCON{}
	if err := s.Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}
}

func TestSpecValidateCommands(t *testing.T) {
	s := validSpec()
	// The zero value (a spec written before the block existed) is valid.
	if err := s.Validate(); err != nil {
		t.Fatalf("zero Commands rejected: %v", err)
	}
	// A fully populated block is valid.
	s.Commands = Commands{MCVersion: "1.20.1", Loader: "forge", Completion: "off"}
	if err := s.Validate(); err != nil {
		t.Fatalf("populated Commands rejected: %v", err)
	}
	if s.Commands.CompletionEnabled() {
		t.Error("CompletionEnabled() should be false when Completion is off")
	}
	s.Commands.Completion = ""
	if !s.Commands.CompletionEnabled() {
		t.Error("CompletionEnabled() should default to true")
	}
	// Two-part versions are fine.
	s.Commands.MCVersion = "1.21"
	if err := s.Validate(); err != nil {
		t.Fatalf("two-part mc_version rejected: %v", err)
	}
}
