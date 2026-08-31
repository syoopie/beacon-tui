package mccmd

import (
	"slices"
	"testing"
)

func TestEmbeddedVersions(t *testing.T) {
	vs := embeddedVersions()
	if len(vs) < 5 {
		t.Fatalf("embedded versions = %v, want the vendored set", vs)
	}
	if !slices.IsSorted(vs) {
		t.Errorf("embedded versions %v are not sorted", vs)
	}
	if !slices.Contains(vs, "1.20.1") {
		t.Errorf("embedded versions %v missing 1.20.1", vs)
	}
	// Every listed version must actually load and parse.
	for _, v := range vs {
		if _, err := loadEmbedded(v); err != nil {
			t.Errorf("loadEmbedded(%q): %v", v, err)
		}
	}
}

func TestBundledSourceTree(t *testing.T) {
	got, err := Bundled().Tree(Context{MCVersion: "1.20.1"})
	if err != nil || got == nil {
		t.Fatalf("Tree(1.20.1) = %v, %v; want a tree", got, err)
	}
	nilTree, err := Bundled().Tree(Context{MCVersion: ""})
	if err != nil || nilTree != nil {
		t.Fatalf("Tree(\"\") = %v, %v; want nil, nil", nilTree, err)
	}
}
