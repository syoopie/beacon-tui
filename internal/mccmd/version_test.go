package mccmd

import "testing"

func TestParseVersion(t *testing.T) {
	ok := map[string]mcVersion{
		"1.20":     {1, 20, 0},
		"1.20.1":   {1, 20, 1},
		" 1.21.4 ": {1, 21, 4},
	}
	for in, want := range ok {
		got, err := parseVersion(in)
		if err != nil || got != want {
			t.Errorf("parseVersion(%q) = %v, %v; want %v", in, got, err, want)
		}
	}
	for _, bad := range []string{"", "1", "1.x", "1.20.1.2", "a.b", "-1.2"} {
		if _, err := parseVersion(bad); err == nil {
			t.Errorf("parseVersion(%q): want error", bad)
		}
	}
}

func TestMatchVersion(t *testing.T) {
	embedded := []string{"1.16.5", "1.18.2", "1.20.1", "1.21.1", "1.21.11"}
	cases := []struct {
		target string
		pick   string
		exact  bool
		minor  bool
	}{
		{"1.20.1", "1.20.1", true, true},
		{"1.20.4", "1.20.1", false, true},   // patch drift within the minor
		{"1.20", "1.20.1", false, true},     // no patch given, same minor exists
		{"1.19.4", "1.18.2", false, false},  // no 1.19 tree: fall back to older
		{"1.21.5", "1.21.1", false, true},   // newest same-minor tree that is not newer
		{"1.99.0", "1.21.11", false, false}, // newer than everything: newest tree
		{"1.12.2", "", false, false},        // older than everything
		{"", "", false, false},
		{"garbage", "", false, false},
	}
	for _, c := range cases {
		got := matchVersion(c.target, embedded)
		if got.pick != c.pick || got.exact != c.exact || got.minor != c.minor {
			t.Errorf("matchVersion(%q) = %+v; want pick=%q exact=%v minor=%v",
				c.target, got, c.pick, c.exact, c.minor)
		}
	}
}
