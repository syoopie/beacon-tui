package server

import "testing"

func TestExecStateExhaustive(t *testing.T) {
	for i := range execStateNames {
		e := ExecState(i)
		name := execStateNames[i]
		if name == "" {
			t.Errorf("ExecState(%d) has no wire name", i)
			continue
		}
		if got := e.String(); got != name {
			t.Errorf("ExecState(%d).String() = %q, want %q", i, got, name)
		}
		parsed, err := ParseExecState(name)
		if err != nil {
			t.Errorf("ParseExecState(%q): %v", name, err)
			continue
		}
		if parsed != e {
			t.Errorf("ParseExecState(%q) = %v, want %v", name, parsed, e)
		}
		text, err := e.MarshalText()
		if err != nil {
			t.Errorf("ExecState(%d).MarshalText(): %v", i, err)
			continue
		}
		var back ExecState
		if err := back.UnmarshalText(text); err != nil {
			t.Errorf("ExecState.UnmarshalText(%q): %v", text, err)
			continue
		}
		if back != e {
			t.Errorf("round trip of %v yielded %v", e, back)
		}
	}
}

func TestParseExecStateRejects(t *testing.T) {
	for _, in := range []string{"sorta", "", "OK", "exec"} {
		if got, err := ParseExecState(in); err == nil {
			t.Errorf("ParseExecState(%q) = %v, want error", in, got)
		}
	}
}

func TestExecStateLaunchable(t *testing.T) {
	for i := range execStateNames {
		e := ExecState(i)
		want := e == ExecOK
		if got := e.Launchable(); got != want {
			t.Errorf("%v.Launchable() = %v, want %v", e, got, want)
		}
	}
}
