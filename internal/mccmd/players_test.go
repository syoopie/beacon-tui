package mccmd

import (
	"slices"
	"testing"
)

func TestPlayerNameCompletion(t *testing.T) {
	e := vanilla(t, "1.20.1")

	// No roster yet: an entity slot has only its usage hint.
	r := e.Complete("kill ", 5)
	if len(r.Suggestions) != 0 {
		t.Fatalf("kill with no roster: suggestions = %v, want none", texts(r.Suggestions))
	}

	e.SetPlayers([]string{"Steve", "Alex", "Alex", " ", "Herobrine"})

	r = e.Complete("kill ", 5)
	if got := texts(r.Suggestions); !slices.Equal(got, []string{"Alex", "Herobrine", "Steve"}) {
		t.Fatalf("kill suggestions = %v, want the sorted deduped roster", got)
	}
	for _, s := range r.Suggestions {
		if s.Kind != SuggestArgument {
			t.Errorf("suggestion %q kind = %d, want SuggestArgument", s.Text, s.Kind)
		}
	}
	if r.Hint == "" {
		t.Error("kill Hint is empty, want the targets usage hint kept alongside the names")
	}

	// Prefix filters, and the replace span covers just the partial token.
	r = e.Complete("kill Al", 7)
	if got := texts(r.Suggestions); !slices.Equal(got, []string{"Alex"}) {
		t.Fatalf("kill Al suggestions = %v, want [Alex]", got)
	}
	if r.Replace != (Span{5, 7}) {
		t.Errorf("Replace = %+v, want {5,7}", r.Replace)
	}
}

func TestPlayerNamesOnlyFillPlayerSlots(t *testing.T) {
	e := vanilla(t, "1.20.1")
	e.SetPlayers([]string{"Steve"})

	// give's second argument is an item, not a player.
	r := e.Complete("give Steve ", len("give Steve "))
	if slices.Contains(texts(r.Suggestions), "Steve") {
		t.Errorf("item slot offered a player name: %v", texts(r.Suggestions))
	}

	// game_profile slots (deop) do take player names.
	r = e.Complete("deop ", 5)
	if !slices.Contains(texts(r.Suggestions), "Steve") {
		t.Errorf("deop suggestions = %v, want Steve", texts(r.Suggestions))
	}
}

func TestSetPlayersClears(t *testing.T) {
	e := vanilla(t, "1.20.1")
	e.SetPlayers([]string{"Steve"})
	e.SetPlayers(nil)
	if r := e.Complete("kill ", 5); len(r.Suggestions) != 0 {
		t.Errorf("after clearing the roster: suggestions = %v, want none", texts(r.Suggestions))
	}
}
