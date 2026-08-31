package mccmd

import (
	"path/filepath"
	"slices"
	"testing"
)

func TestHistoryAddDedupesAndCaps(t *testing.T) {
	h := NewHistory(3)
	h.Add("list")
	h.Add("list") // identical to the current newest: dropped
	h.Add(" list ")
	h.Add("time set day")
	if got := h.All(); !slices.Equal(got, []string{"list", "time set day"}) {
		t.Fatalf("after dedupe, history = %v", got)
	}
	h.Add("weather clear")
	h.Add("say hi")
	h.Add("stop")
	if got := h.All(); !slices.Equal(got, []string{"weather clear", "say hi", "stop"}) {
		t.Fatalf("history did not cap to 3: %v", got)
	}
	h.Add("")
	h.Add("   ")
	if h.Len() != 3 {
		t.Fatalf("blank lines changed the history: len = %d", h.Len())
	}
}

func TestHistorySaveLoadRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "smp.history")

	h := NewHistory(0)
	for _, line := range []string{"list", "time set day", "gamerule keepInventory true"} {
		h.Add(line)
	}
	if err := h.Save(path); err != nil {
		t.Fatalf("Save: %v", err)
	}

	back, err := LoadHistory(path, 0)
	if err != nil {
		t.Fatalf("LoadHistory: %v", err)
	}
	if !slices.Equal(back.All(), h.All()) {
		t.Fatalf("round trip: got %v, want %v", back.All(), h.All())
	}
}

func TestLoadHistoryMissingFile(t *testing.T) {
	h, err := LoadHistory(filepath.Join(t.TempDir(), "absent"), 10)
	if err != nil {
		t.Fatalf("LoadHistory on a missing file: %v", err)
	}
	if h.Len() != 0 {
		t.Fatalf("missing file should load empty, got %d lines", h.Len())
	}
}
