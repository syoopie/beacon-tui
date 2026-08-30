package server

import "testing"

func TestSessionFor(t *testing.T) {
	got := SessionFor("survival")
	if want := Session("beacon-survival"); got != want {
		t.Fatalf("SessionFor = %q, want %q", got, want)
	}
	if got.String() != "beacon-survival" {
		t.Fatalf("String = %q", got.String())
	}
	text, err := got.MarshalText()
	if err != nil {
		t.Fatalf("MarshalText: %v", err)
	}
	var back Session
	if err := back.UnmarshalText(text); err != nil {
		t.Fatalf("UnmarshalText(%q): %v", text, err)
	}
	if back != got {
		t.Fatalf("round trip yielded %q", back)
	}
}

func TestSessionUnmarshalTextRejects(t *testing.T) {
	tests := []struct {
		name string
		in   string
	}{
		{"no prefix", "survival"},
		{"prefix only", "beacon-"},
		{"empty", ""},
		{"whitespace", "beacon-survival world"},
		{"tab", "beacon-survival\tworld"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var s Session
			if err := s.UnmarshalText([]byte(tt.in)); err == nil {
				t.Fatalf("UnmarshalText(%q) accepted, want error", tt.in)
			}
		})
	}
}
