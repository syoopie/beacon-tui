package mccmd

import (
	"slices"
	"strings"
	"testing"
)

func vanilla(t *testing.T, version string) *Engine {
	t.Helper()
	e, err := New(Options{
		Sources: []VocabularySource{Bundled()},
		Context: Context{MCVersion: version, ServerID: "smp"},
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return e
}

func texts(sugs []Suggestion) []string {
	out := make([]string, len(sugs))
	for i, s := range sugs {
		out[i] = s.Text
	}
	return out
}

func TestCompleteCommandName(t *testing.T) {
	e := vanilla(t, "1.20.1")

	r := e.Complete("gam", 3)
	if !slices.Contains(texts(r.Suggestions), "gamerule") {
		t.Errorf("Complete(%q) suggestions = %v, want gamerule", "gam", texts(r.Suggestions))
	}
	if r.Replace != (Span{0, 3}) {
		t.Errorf("Replace = %+v, want {0,3}", r.Replace)
	}

	// A leading slash is optional and must not shift the replace span onto it.
	r = e.Complete("/gam", 4)
	if !slices.Contains(texts(r.Suggestions), "gamerule") {
		t.Errorf("Complete(%q) = %v, want gamerule", "/gam", texts(r.Suggestions))
	}
	if r.Replace != (Span{1, 4}) {
		t.Errorf("Replace = %+v, want {1,4}", r.Replace)
	}
}

func TestCompleteEmptyListsCommands(t *testing.T) {
	e := vanilla(t, "1.20.1")
	r := e.Complete("", 0)
	got := texts(r.Suggestions)
	for _, want := range []string{"gamerule", "give", "time", "weather"} {
		if !slices.Contains(got, want) {
			t.Errorf("Complete(%q) missing %q", "", want)
		}
	}
	if r.Hint != "" {
		t.Errorf("Hint at root = %q, want empty", r.Hint)
	}
}

func TestCompleteSubcommandLiteral(t *testing.T) {
	e := vanilla(t, "1.20.1")

	r := e.Complete("gamerule keepInv", len("gamerule keepInv"))
	if got := texts(r.Suggestions); !slices.Contains(got, "keepInventory") {
		t.Errorf("suggestions = %v, want keepInventory", got)
	}
	if r.Replace != (Span{9, 16}) {
		t.Errorf("Replace = %+v, want {9,16}", r.Replace)
	}

	r = e.Complete("gamerule ", 9)
	if len(r.Suggestions) < 20 {
		t.Errorf("gamerule has %d suggestions, want the full rule list", len(r.Suggestions))
	}
}

func TestCompleteUsageHint(t *testing.T) {
	e := vanilla(t, "1.20.1")

	r := e.Complete("gamerule keepInventory ", len("gamerule keepInventory "))
	if len(r.Suggestions) != 0 {
		t.Errorf("no literal follows a rule name, got %v", texts(r.Suggestions))
	}
	if r.Hint != "[<value>]" {
		t.Errorf("Hint = %q, want [<value>]", r.Hint)
	}

	r = e.Complete("give ", 5)
	if !strings.Contains(r.Hint, "<targets>") {
		t.Errorf("give Hint = %q, want it to mention <targets>", r.Hint)
	}
}

func TestCompleteFollowsExecuteRedirect(t *testing.T) {
	e := vanilla(t, "1.20.1")

	r := e.Complete("execute ", 8)
	for _, want := range []string{"as", "at", "run", "if"} {
		if !slices.Contains(texts(r.Suggestions), want) {
			t.Errorf("execute suggestions = %v, want %q", texts(r.Suggestions), want)
		}
	}

	// After "run" the grammar redirects to the root: full command set again.
	r = e.Complete("execute run ", len("execute run "))
	for _, want := range []string{"gamerule", "give"} {
		if !slices.Contains(texts(r.Suggestions), want) {
			t.Errorf("execute run suggestions = %v, want %q", texts(r.Suggestions), want)
		}
	}
}

func TestCompleteUnknown(t *testing.T) {
	e := vanilla(t, "1.20.1")

	r := e.Complete("wobble", 6)
	if len(r.Suggestions) != 0 || !strings.Contains(r.Hint, "unknown command") {
		t.Errorf("Complete(wobble) = %v / %q, want no suggestions and an unknown-command hint", texts(r.Suggestions), r.Hint)
	}

	r = e.Complete("gamerule wobble extra", len("gamerule wobble extra"))
	if !strings.Contains(r.Hint, "unexpected") {
		t.Errorf("Hint = %q, want an unexpected-token hint", r.Hint)
	}
}

func TestDegraded(t *testing.T) {
	cases := []struct {
		version string
		want    string
	}{
		{"1.20.1", ""},
		{"1.20.4", ""}, // patch drift is fine
		{"", "set mc_version"},
		{"1.12.2", "no command data for 1.12.2"},
		{"1.99.0", "using 1.21.11"},
	}
	for _, c := range cases {
		e := vanilla(t, c.version)
		switch {
		case c.want == "" && e.Degraded() != "":
			t.Errorf("version %q: Degraded = %q, want none", c.version, e.Degraded())
		case c.want != "" && !strings.Contains(e.Degraded(), c.want):
			t.Errorf("version %q: Degraded = %q, want it to contain %q", c.version, e.Degraded(), c.want)
		}
		// A degraded engine still answers, it just may have nothing to say.
		_ = e.Complete("g", 1)
	}
}

func TestEmptyEngineIsUsable(t *testing.T) {
	e, err := New(Options{Context: Context{MCVersion: "1.20.1"}})
	if err != nil {
		t.Fatalf("New with no sources: %v", err)
	}
	r := e.Complete("gamerule", 8)
	if len(r.Suggestions) != 0 {
		t.Errorf("no sources should mean no suggestions, got %v", texts(r.Suggestions))
	}
}
