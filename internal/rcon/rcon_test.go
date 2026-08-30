package rcon

import (
	"reflect"
	"testing"
)

func TestParseList(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want Snapshot
	}{
		{
			name: "modern with players",
			in:   "There are 2 of a max of 20 players online: Steve, Alex",
			want: Snapshot{Online: 2, Max: 20, Players: []string{"Steve", "Alex"}},
		},
		{
			name: "modern empty",
			in:   "There are 0 of a max of 20 players online:",
			want: Snapshot{Online: 0, Max: 20},
		},
		{
			name: "legacy slash form",
			in:   "There are 1/10 players online: Notch",
			want: Snapshot{Online: 1, Max: 10, Players: []string{"Notch"}},
		},
		{
			name: "colour codes stripped",
			in:   "There are 1 of a max of 5 players online: §aSteve",
			want: Snapshot{Online: 1, Max: 5, Players: []string{"Steve"}},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := parseList(c.in)
			if err != nil {
				t.Fatalf("parseList: %v", err)
			}
			if !reflect.DeepEqual(got, c.want) {
				t.Fatalf("parseList(%q) = %+v, want %+v", c.in, got, c.want)
			}
		})
	}
}

func TestParseListRejectsGarbage(t *testing.T) {
	if _, err := parseList("Unknown command. Try /help for a list of commands"); err == nil {
		t.Fatal("expected an error for a non-list response")
	}
}
