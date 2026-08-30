package importdetect

import (
	"maps"
	"reflect"
	"testing"

	"github.com/sunyupei/beacon-tui/internal/config"
	"github.com/sunyupei/beacon-tui/internal/server"
)

func TestBuildSpecs(t *testing.T) {
	dirs := config.Dirs{Config: "/cfg/beacon", State: "/state/beacon"}
	cands := []Candidate{
		{
			Dir:    "/srv/a/survival",
			Base:   "survival",
			Start:  "./run.sh",
			Script: "run.sh",
			Exec:   server.ExecOK,
			Port:   25565,
		},
		{
			Dir:   "/srv/b/survival",
			Base:  "survival",
			Start: "java -jar server.jar nogui",
			Exec:  server.ExecOK,
			Port:  25566,
		},
	}

	tests := []struct {
		name  string
		taken map[server.ID]bool
		want  []server.ID
	}{
		{"nil taken", nil, []server.ID{"survival", "survival-2"}},
		{"empty taken", map[server.ID]bool{}, []server.ID{"survival", "survival-2"}},
		{"survival already taken", map[server.ID]bool{"survival": true}, []server.ID{"survival-2", "survival-3"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			before := maps.Clone(tt.taken)

			got := BuildSpecs(dirs, cands, tt.taken)

			if len(got) != len(tt.want) {
				t.Fatalf("BuildSpecs returned %d specs, want %d", len(got), len(tt.want))
			}
			for i, spec := range got {
				id := tt.want[i]
				if spec.ID != id {
					t.Errorf("spec %d id = %q, want %q", i, spec.ID, id)
				}
				if spec.Session != server.SessionFor(id) {
					t.Errorf("spec %d session = %q, want %q", i, spec.Session, server.SessionFor(id))
				}
				if spec.LogFile != dirs.LogFile(id) {
					t.Errorf("spec %d log file = %q, want %q", i, spec.LogFile, dirs.LogFile(id))
				}
				if spec.Dir != cands[i].Dir || spec.Start != cands[i].Start ||
					spec.Script != cands[i].Script || spec.Port != cands[i].Port || spec.Exec != cands[i].Exec {
					t.Errorf("spec %d = %+v, want the fields of %+v", i, spec, cands[i])
				}
				if spec.State.LastKnown != server.StatusStopped {
					t.Errorf("spec %d last known = %v, want stopped", i, spec.State.LastKnown)
				}
				if err := spec.Validate(); err != nil {
					t.Errorf("spec %d rejected by Validate: %v", i, err)
				}
			}

			if len(tt.taken) != len(before) || !reflect.DeepEqual(tt.taken, before) {
				t.Fatalf("taken = %v after the call, want %v", tt.taken, before)
			}
		})
	}
}

func TestBuildSpecsSessionPrefix(t *testing.T) {
	dirs := config.Dirs{Config: "/cfg/beacon", State: "/state/beacon"}
	cands := []Candidate{{Dir: "/srv/creative", Base: "creative", Start: "./run.sh", Script: "run.sh", Port: 25565}}

	got := BuildSpecs(dirs, cands, nil)

	if len(got) != 1 {
		t.Fatalf("BuildSpecs returned %d specs, want 1", len(got))
	}
	if got[0].Session != "beacon-creative" {
		t.Fatalf("session = %q, want %q", got[0].Session, "beacon-creative")
	}
	if got[0].LogFile != "/state/beacon/logs/creative.log" {
		t.Fatalf("log file = %q, want %q", got[0].LogFile, "/state/beacon/logs/creative.log")
	}
}
