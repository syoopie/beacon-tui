package mccmd

import "strings"

// formatUsage renders what may follow pos as a Brigadier-style usage string:
// "<rule> [<value>]" for gamerule, "(grant|revoke) <targets> …" for
// advancement. It walks a single deterministic path forward and stops at the
// first real branch, so the string stays short; the suggestion list carries the
// breadth. The result is truncated to budget runes.
func formatUsage(root, pos *Node, budget int) string {
	s := strings.TrimSpace(usageChain(root, pos, 6))
	return truncateRunes(s, budget)
}

// usageChain describes pos's continuation and, when that continuation is a
// single unambiguous step, the step after it, up to depth levels.
func usageChain(root, n *Node, depth int) string {
	if depth <= 0 {
		return "…"
	}
	lits := n.literalChildren()
	arg := n.argChild()

	switch {
	case len(lits) == 0 && arg == nil:
		return "" // nothing more to type

	case arg != nil && len(lits) == 0:
		tok := "<" + arg.Name + ">"
		rest := usageChain(root, resolve(root, arg), depth-1)
		return optional(n, join(tok, rest))

	case len(lits) > 0 && arg == nil && len(lits) == 1:
		tok := lits[0].Name
		rest := usageChain(root, resolve(root, lits[0]), depth-1)
		return optional(n, join(tok, rest))

	default:
		// A real branch: list the options and stop. The suggestion box is
		// where the user picks one.
		var names []string
		for _, l := range lits {
			names = append(names, l.Name)
		}
		if arg != nil {
			names = append(names, "<"+arg.Name+">")
		}
		return optional(n, "("+strings.Join(names, "|")+")")
	}
}

// optional wraps s in brackets when n is itself a valid place to stop, marking
// everything past it as not required.
func optional(n *Node, s string) string {
	if s == "" {
		return ""
	}
	if n.Executable {
		return "[" + s + "]"
	}
	return s
}

func join(a, b string) string {
	if b == "" {
		return a
	}
	return a + " " + b
}

func truncateRunes(s string, budget int) string {
	if budget <= 0 {
		return s
	}
	r := []rune(s)
	if len(r) <= budget {
		return s
	}
	if budget == 1 {
		return "…"
	}
	return string(r[:budget-1]) + "…"
}
