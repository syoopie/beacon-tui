package mccmd

import (
	"os"
	"path/filepath"
	"testing"
)

func readFixture(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestParseHelpVanillaAndModdedCommands(t *testing.T) {
	root := parseHelp(readFixture(t, "help_forge_1.20.1.txt"), "forge")
	if root == nil {
		t.Fatal("parseHelp returned nil for a real /help dump")
	}

	for _, name := range []string{"advancement", "give", "forge", "ftbquests", "twilightforest", "spark"} {
		if root.Children[name] == nil {
			t.Errorf("command %q missing from the parsed tree", name)
		}
	}

	// "/forge (tps|track|entity|generate|dimensions|mods|tags)" -> literal
	// children, each a usable leaf.
	forge := root.Children["forge"]
	if forge.Children["tps"] == nil || !forge.Children["tps"].Executable {
		t.Fatalf("forge subcommands not parsed: %+v", forge.childNames())
	}

	// "/give <targets> <item> [<count>]" -> an argument chain, not literals.
	give := root.Children["give"]
	if give.argChild() == nil {
		t.Fatalf("give should take an argument, got children %v", give.childNames())
	}
}

func TestParseHelpAliasBecomesRedirect(t *testing.T) {
	root := parseHelp("/tp -> teleport\n/teleport (<location>|<destination>|<targets>)\n", "vanilla")
	tp := root.Children["tp"]
	if tp == nil || len(tp.Redirect) != 1 || tp.Redirect[0] != "teleport" {
		t.Fatalf("tp alias not a redirect to teleport: %+v", tp)
	}
	if got := resolve(root, tp); got != root.Children["teleport"] {
		t.Fatalf("resolving the tp alias did not reach teleport")
	}
}

func TestParseHelpOptionalLiteralChoice(t *testing.T) {
	root := parseHelp("/difficulty [peaceful|easy|normal|hard]\n", "vanilla")
	d := root.Children["difficulty"]
	if !d.Executable {
		t.Error("an optional-only argument should leave the command executable on its own")
	}
	if d.Children["easy"] == nil {
		t.Errorf("difficulty choices not parsed: %v", d.childNames())
	}
}

func TestHelpSourceEmptyContributesNothing(t *testing.T) {
	tree, err := HelpSource("   ").Tree(Context{})
	if err != nil || tree != nil {
		t.Fatalf("empty help text should contribute nothing, got %v %v", tree, err)
	}
}

func TestHelpSourceMergesUnderBundled(t *testing.T) {
	eng, err := New(Options{
		Sources: []VocabularySource{Bundled(), HelpSource(readFixture(t, "help_forge_1.20.1.txt"))},
		Context: Context{MCVersion: "1.20.1", Loader: "forge"},
	})
	if err != nil {
		t.Fatal(err)
	}

	// A modded command the bundled tree cannot know is now completable.
	res := eng.Complete("ftbques", 7)
	if len(res.Suggestions) == 0 || res.Suggestions[0].Text != "ftbquests" {
		t.Fatalf("ftbquests not suggested from the merged tree: %+v", res.Suggestions)
	}

	// The bundled grammar for a shared command is intact: /give still walks to
	// its real argument chain, not the coarse /help one.
	give := eng.Complete("give ", 5)
	if give.Hint == "" {
		t.Fatalf("give lost its usage hint after the merge")
	}
}
