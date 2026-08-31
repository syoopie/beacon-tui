package mccmd

import "testing"

func lit(name string, children ...*Node) *Node {
	n := &Node{Kind: KindLiteral, Name: name}
	if len(children) > 0 {
		n.Children = map[string]*Node{}
		for _, c := range children {
			n.Children[c.Name] = c
		}
	}
	return n
}

func TestResolveRedirect(t *testing.T) {
	root := &Node{Kind: KindRoot, Children: map[string]*Node{}}
	execute := lit("execute")
	root.Children["execute"] = execute
	execute.Children = map[string]*Node{
		"as":  {Kind: KindArgument, Name: "targets", Redirect: []string{"execute"}},
		"run": {Kind: KindLiteral, Name: "run"}, // the bare run-to-root shape
	}

	if got := resolve(root, execute.Children["as"]); got != execute {
		t.Errorf("resolve(execute as) = %v, want the execute node", got)
	}
	if got := resolve(root, execute.Children["run"]); got != root {
		t.Errorf("resolve(execute run) = %v, want root", got)
	}
	if got := resolve(root, &Node{Kind: KindLiteral, Name: "x", Redirect: []string{}}); got != root {
		t.Errorf("resolve(empty redirect) = %v, want root", got)
	}
}

func TestMergeNodes(t *testing.T) {
	hi := &Node{Kind: KindRoot, Children: map[string]*Node{
		"give": lit("give"),
	}}
	lo := &Node{Kind: KindRoot, Children: map[string]*Node{
		"give":      lit("give", lit("mine")), // extra child only lo knows
		"ftbquests": lit("ftbquests"),         // a modded command only lo has
	}}

	got := mergeNodes(hi, lo)
	if got.Children["ftbquests"] == nil {
		t.Error("merge dropped the command only the low-priority source had")
	}
	if got.Children["give"].Children["mine"] == nil {
		t.Error("merge did not union children of a shared command")
	}
}

func TestMergeNodesPrefersHighPriorityParser(t *testing.T) {
	hi := &Node{Kind: KindArgument, Name: "x", Parser: "brigadier:bool"}
	lo := &Node{Kind: KindArgument, Name: "x", Parser: "brigadier:string", Executable: true}
	got := mergeNodes(hi, lo)
	if got.Parser != "brigadier:bool" {
		t.Errorf("parser = %q, want the high-priority source's", got.Parser)
	}
	if !got.Executable {
		t.Error("executable should be the OR of both sources")
	}
}
