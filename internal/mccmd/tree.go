package mccmd

import "sort"

// Kind is a command-tree node's role, matching the "type" field of
// commands.json.
type Kind uint8

const (
	KindRoot Kind = iota
	KindLiteral
	KindArgument
)

// Node is one node of the command tree. It mirrors a node of Mojang's generated
// commands.json so a report parses with no translation and a test fixture is
// just the real file. Nodes are treated as immutable once built; [mergeNodes]
// returns fresh nodes rather than mutating either input.
type Node struct {
	Kind       Kind
	Name       string // literal text, or argument name; "" for the root
	Parser     string // argument nodes: "brigadier:bool", "minecraft:entity", …; "" means opaque
	Properties map[string]any
	Children   map[string]*Node // keyed by Name
	Executable bool             // a valid command may terminate here
	// Redirect is the path from the root whose children continue this node
	// (commands.json spells "execute as <t> …" this way). Nil unless set.
	Redirect []string
}

// childNames returns this node's child keys in a stable order.
func (n *Node) childNames() []string {
	if n == nil || len(n.Children) == 0 {
		return nil
	}
	names := make([]string, 0, len(n.Children))
	for k := range n.Children {
		names = append(names, k)
	}
	sort.Strings(names)
	return names
}

// literalChildren returns this node's literal children, ordered.
func (n *Node) literalChildren() []*Node {
	var out []*Node
	for _, name := range n.childNames() {
		if c := n.Children[name]; c.Kind == KindLiteral {
			out = append(out, c)
		}
	}
	return out
}

// argChild returns this node's single argument child, if it has one. A vanilla
// node never mixes an argument child with literal children, and never carries
// more than one argument child.
func (n *Node) argChild() *Node {
	for _, name := range n.childNames() {
		if c := n.Children[name]; c.Kind == KindArgument {
			return c
		}
	}
	return nil
}

// isRunRedirect reports whether n is a bare node that redirects to the root:
// "execute run" and its kin are emitted as a childless, non-executable literal
// with no explicit redirect. Every other leaf is either executable or carries a
// Redirect, so this shape is unambiguous.
func (n *Node) isRunRedirect() bool {
	return n.Kind != KindRoot && !n.Executable && len(n.Children) == 0 && n.Redirect == nil
}

// resolve returns the node whose children actually continue n: n itself
// normally, or the redirect target when n redirects. Root redirects (the "run"
// shape and an explicit empty path) resolve to root. Bounded so a malformed
// tree cannot loop.
func resolve(root, n *Node) *Node {
	for hops := 0; hops < 8; hops++ {
		switch {
		case n.isRunRedirect():
			return root
		case len(n.Redirect) == 0 && n.Redirect != nil:
			return root
		case len(n.Redirect) > 0:
			t := root
			for _, seg := range n.Redirect {
				next := t.Children[seg]
				if next == nil {
					return n // broken redirect: fall back to the node as written
				}
				t = next
			}
			n = t
		default:
			return n
		}
	}
	return root
}

// mergeNodes overlays lo beneath hi: hi's Parser, Executable and Redirect win
// where hi sets them, and children are unioned, recursing on shared names. Used
// to fold a lower-priority source (a coarse RCON /help tree, in a later phase)
// under the bundled vanilla tree without losing either's commands.
func mergeNodes(hi, lo *Node) *Node {
	if hi == nil {
		return lo
	}
	if lo == nil {
		return hi
	}
	out := &Node{
		Kind:       hi.Kind,
		Name:       hi.Name,
		Parser:     hi.Parser,
		Properties: hi.Properties,
		Executable: hi.Executable || lo.Executable,
		Redirect:   hi.Redirect,
	}
	if out.Parser == "" {
		out.Parser = lo.Parser
		out.Properties = lo.Properties
	}
	if out.Redirect == nil {
		out.Redirect = lo.Redirect
	}
	if len(hi.Children) == 0 && len(lo.Children) == 0 {
		return out
	}
	out.Children = make(map[string]*Node, len(hi.Children)+len(lo.Children))
	for name, c := range lo.Children {
		out.Children[name] = c
	}
	for name, c := range hi.Children {
		out.Children[name] = mergeNodes(c, out.Children[name])
	}
	return out
}
