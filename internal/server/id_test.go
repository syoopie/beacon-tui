package server

import (
	"strings"
	"testing"
)

func TestParseID(t *testing.T) {
	tests := []struct {
		name    string
		in      string
		wantErr bool
	}{
		{"simple", "survival", false},
		{"digits and dashes", "creative-2", false},
		{"underscore", "my_server", false},
		{"max length", strings.Repeat("a", 64), false},
		{"empty", "", true},
		{"dot", ".", true},
		{"dotdot", "..", true},
		{"uppercase", "Survival", true},
		{"space", "survival world", true},
		{"slash", "srv/survival", true},
		{"too long", strings.Repeat("a", 65), true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseID(tt.in)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("ParseID(%q) = %q, want error", tt.in, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseID(%q): %v", tt.in, err)
			}
			if got.String() != tt.in {
				t.Fatalf("ParseID(%q) = %q", tt.in, got)
			}
		})
	}
}

func TestIDFromDir(t *testing.T) {
	tests := []struct {
		name    string
		in      string
		want    ID
		wantErr bool
	}{
		{"spaces and case", "/srv/mc/Survival World", "survival-world", false},
		{"already clean", "/srv/mc/survival", "survival", false},
		{"trailing slash", "/srv/mc/Creative/", "creative", false},
		{"punctuation run", "/srv/mc/Paper 1.20.4!!", "paper-1-20-4", false},
		{"underscore kept", "/srv/mc/my_server", "my_server", false},
		{"root", "/", "", true},
		{"nothing usable", "/srv/mc/!!!", "", true},
		{"long name truncated", "/srv/mc/" + strings.Repeat("a", 80), ID(strings.Repeat("a", 64)), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := IDFromDir(tt.in)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("IDFromDir(%q) = %q, want error", tt.in, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("IDFromDir(%q): %v", tt.in, err)
			}
			if got != tt.want {
				t.Fatalf("IDFromDir(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestNextFreeID(t *testing.T) {
	taken := map[ID]bool{}
	for _, want := range []ID{"survival", "survival-2", "survival-3"} {
		got := NextFreeID("survival", taken)
		if got != want {
			t.Fatalf("NextFreeID = %q, want %q", got, want)
		}
		taken[got] = true
	}
}

func TestNextFreeIDStaysParseable(t *testing.T) {
	base := ID(strings.Repeat("a", 64))
	taken := map[ID]bool{base: true}
	got := NextFreeID(base, taken)
	if _, err := ParseID(string(got)); err != nil {
		t.Fatalf("NextFreeID(%q) = %q: %v", base, got, err)
	}
}

func TestIDUnmarshalText(t *testing.T) {
	var id ID
	if err := id.UnmarshalText([]byte("survival")); err != nil {
		t.Fatalf("UnmarshalText: %v", err)
	}
	if id != "survival" {
		t.Fatalf("id = %q", id)
	}
	if err := id.UnmarshalText([]byte("Survival World")); err == nil {
		t.Fatal("UnmarshalText accepted an invalid id")
	}
}
