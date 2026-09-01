package mccmd

import "strings"

// helpSource contributes the commands a running server lists under "/help",
// parsed from the raw text the UI fetched over RCON. It fills in the modded
// commands the bundled vanilla tree cannot know about ("/forge", "/ftbquests"),
// each one level deep: "/help" prints a command's first line of usage, so a
// group like "(tps|track|entity)" becomes literal children but a deeper tree
// needs a per-command "/help <name>" that a later phase can add.
//
// Priority is below [bundledSource] on purpose. Where a command exists in both,
// the bundled grammar is the accurate one and wins; the /help tree only adds
// what bundled is missing.
type helpSource struct {
	raw string
}

// HelpSource parses raw "/help" output into a [VocabularySource]. The UI fetches
// the text off the render path and re-creates the source when it changes, the
// caching contract [VocabularySource] describes. Empty raw contributes nothing.
func HelpSource(raw string) VocabularySource { return &helpSource{raw: raw} }

func (h *helpSource) Name() string  { return "rcon-help" }
func (h *helpSource) Priority() int { return -10 }

func (h *helpSource) Tree(ctx Context) (*Node, error) {
	if strings.TrimSpace(h.raw) == "" {
		return nil, nil
	}
	return parseHelp(h.raw, ctx.Loader), nil
}

// parseHelp turns "/help" output into a root whose children are the listed
// commands. loader selects the dialect; only the vanilla/Forge format (one
// "/command <usage>" per line) is handled today, which is also what a Fabric or
// Quilt server prints. Paper's plugin-grouped format is a later phase.
func parseHelp(raw, loader string) *Node {
	root := &Node{Kind: KindRoot, Children: map[string]*Node{}}
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "/") {
			continue
		}
		name, rest, _ := strings.Cut(line[1:], " ")
		if name == "" {
			continue
		}

		// "/tp -> teleport": an alias. Model it as a redirect so walking it
		// lands on the target's children.
		if target, ok := strings.CutPrefix(strings.TrimSpace(rest), "-> "); ok {
			root.Children[name] = &Node{Kind: KindLiteral, Name: name, Redirect: []string{strings.TrimSpace(target)}}
			continue
		}

		node := root.Children[name]
		if node == nil {
			node = &Node{Kind: KindLiteral, Name: name, Children: map[string]*Node{}}
			root.Children[name] = node
		}
		applyUsage(node, usageTokens(strings.TrimSpace(rest)))
	}
	if len(root.Children) == 0 {
		return nil
	}
	return root
}

// usageTokens splits a usage string on spaces while keeping a bracketed group
// whole: "showfile <mod> <type>" -> ["showfile", "<mod>", "<type>"],
// "(tps|track|entity list [<filter>])" stays one token.
func usageTokens(s string) []string {
	var toks []string
	var cur strings.Builder
	depth := 0
	flush := func() {
		if cur.Len() > 0 {
			toks = append(toks, cur.String())
			cur.Reset()
		}
	}
	for _, r := range s {
		switch r {
		case '(', '[', '<':
			depth++
			cur.WriteRune(r)
		case ')', ']', '>':
			if depth > 0 {
				depth--
			}
			cur.WriteRune(r)
		case ' ':
			if depth == 0 {
				flush()
			} else {
				cur.WriteRune(r)
			}
		default:
			cur.WriteRune(r)
		}
	}
	flush()
	return toks
}

// applyUsage grows node by one usage line's worth of children. A chain of bare
// words and <args> is walked node by node; a choice group "(a|b|c)" or "[a|b]"
// becomes children and ends the line, because /help does not say what follows
// each branch.
func applyUsage(node *Node, toks []string) {
	cur := node
	for _, tok := range toks {
		switch {
		case strings.HasPrefix(tok, "<") && strings.HasSuffix(tok, ">"):
			cur = childArg(cur, strings.Trim(tok, "<>"))

		case strings.HasPrefix(tok, "(") || strings.HasPrefix(tok, "["):
			optional := tok[0] == '['
			alts := strings.Split(tok[1:len(tok)-1], "|")
			if anyContains(alts, "<") {
				// "(<location>|<destination>|<targets>)": alternative argument
				// forms, not literals. One opaque slot is the best we can do.
				childArg(cur, "arg")
			} else {
				for _, a := range alts {
					if a = strings.TrimSpace(a); a != "" {
						childLiteral(cur, a).Executable = true
					}
				}
			}
			if optional {
				cur.Executable = true
			}
			return

		default:
			cur = childLiteral(cur, tok)
		}
	}
	cur.Executable = true
}

func childLiteral(parent *Node, name string) *Node {
	if parent.Children == nil {
		parent.Children = map[string]*Node{}
	}
	if c := parent.Children[name]; c != nil {
		return c
	}
	c := &Node{Kind: KindLiteral, Name: name, Children: map[string]*Node{}}
	parent.Children[name] = c
	return c
}

func childArg(parent *Node, name string) *Node {
	if parent.Children == nil {
		parent.Children = map[string]*Node{}
	}
	if a := parent.argChild(); a != nil {
		return a
	}
	a := &Node{Kind: KindArgument, Name: name, Children: map[string]*Node{}}
	parent.Children[name] = a
	return a
}

func anyContains(ss []string, sub string) bool {
	for _, s := range ss {
		if strings.Contains(s, sub) {
			return true
		}
	}
	return false
}
