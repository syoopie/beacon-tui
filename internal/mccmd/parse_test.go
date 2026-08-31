package mccmd

import (
	"strings"
	"testing"
)

func TestParseRejectsJunk(t *testing.T) {
	if _, err := Parse(strings.NewReader(`{"type":"literal"}`)); err == nil {
		t.Fatal("Parse: want error when the root is not type root")
	}
	if _, err := Parse(strings.NewReader(`not json`)); err == nil {
		t.Fatal("Parse: want error on non-JSON")
	}
	if _, err := Parse(strings.NewReader(`{"type":"root","children":{"x":{"type":"weird"}}}`)); err == nil {
		t.Fatal("Parse: want error on an unknown node type")
	}
}

func TestParseEmbeddedTree(t *testing.T) {
	root, err := loadEmbedded("1.20.1")
	if err != nil {
		t.Fatalf("loadEmbedded: %v", err)
	}
	if root.Kind != KindRoot {
		t.Fatalf("root kind = %v, want KindRoot", root.Kind)
	}
	if len(root.Children) < 50 {
		t.Fatalf("root has %d children, want a full vanilla set", len(root.Children))
	}

	gr := root.Children["gamerule"]
	if gr == nil || gr.Kind != KindLiteral {
		t.Fatal("no gamerule literal in the tree")
	}
	if gr.Children["keepInventory"] == nil {
		t.Fatal("gamerule has no keepInventory child")
	}

	give := root.Children["give"]
	if give == nil || give.argChild() == nil || give.argChild().Parser != "minecraft:entity" {
		t.Fatalf("give's first argument should parse minecraft:entity, got %+v", give.argChild())
	}

	// execute carries redirect nodes back to itself.
	exec := root.Children["execute"]
	if exec == nil {
		t.Fatal("no execute command")
	}
	as := exec.Children["as"]
	if as == nil || as.argChild() == nil || len(as.argChild().Redirect) == 0 {
		t.Fatalf("execute as <targets> should redirect, got %+v", as)
	}
}
