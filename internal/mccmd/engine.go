package mccmd

import (
	"fmt"
	"sort"
	"strings"
)

// Options configure a new [Engine].
type Options struct {
	Sources []VocabularySource
	Context Context
}

// Engine holds the merged command tree for one server and answers completion
// queries against it. It implements [Completer]. Build it once per selected
// server; Complete is pure and cheap to call on every keystroke.
type Engine struct {
	root     *Node
	degraded string
}

// New folds every source's tree into one, highest [VocabularySource.Priority]
// winning conflicts. An Engine with nothing to say is still valid: Complete
// yields no suggestions and [Engine.Degraded] explains why.
func New(opts Options) (*Engine, error) {
	sources := append([]VocabularySource(nil), opts.Sources...)
	sort.SliceStable(sources, func(i, j int) bool {
		return sources[i].Priority() > sources[j].Priority()
	})

	e := &Engine{root: &Node{Kind: KindRoot}}
	contributed := false
	for _, s := range sources {
		tree, err := s.Tree(opts.Context)
		if err != nil {
			return nil, fmt.Errorf("mccmd: source %q: %w", s.Name(), err)
		}
		if tree == nil {
			continue
		}
		e.root = mergeNodes(e.root, tree)
		contributed = true
	}

	e.degraded = degradedReason(opts.Context, contributed)
	return e, nil
}

// degradedReason names, in words a server owner can act on, why completion is
// off or weaker than it should be. Empty when completion is at full strength.
func degradedReason(ctx Context, contributed bool) string {
	if contributed && ctx.MCVersion != "" {
		m := matchVersion(ctx.MCVersion, embeddedVersions())
		if m.pick != "" && !m.exact && !m.minor {
			return fmt.Sprintf("completion: no command data for %s, using %s", ctx.MCVersion, m.pick)
		}
		return ""
	}
	if ctx.MCVersion == "" {
		where := "servers/<id>.toml"
		if ctx.ServerID != "" {
			where = "servers/" + ctx.ServerID + ".toml"
		}
		return "completion off: set mc_version in " + where
	}
	if !contributed {
		return fmt.Sprintf("completion off: no command data for %s", ctx.MCVersion)
	}
	return ""
}

// Degraded is a one-line note for the console to show when completion is off or
// running on a mismatched command tree. Empty means all is well.
func (e *Engine) Degraded() string { return e.degraded }

// Complete answers "given this console input with the cursor here, what can be
// typed next, and what does the command under the cursor take". Text after the
// cursor is ignored, the way a shell completes.
func (e *Engine) Complete(line string, cursor int) Result {
	res := Result{Degraded: e.degraded}
	if cursor < 0 || cursor > len(line) {
		cursor = len(line)
	}
	head := line[:cursor]

	// A single leading slash is optional in the beacon console and in the
	// game; drop it before walking the tree.
	head = strings.TrimPrefix(head, "/")

	done, partial := splitCommand(head)
	res.Replace = Span{Start: cursor - len(partial), End: cursor}

	pos, ok := e.walk(done)
	if !ok {
		res.Hint = unknownHint(pos, done)
		return res
	}

	res.Suggestions = literalSuggestions(pos, partial)

	switch {
	case len(res.Suggestions) == 0 && partial != "" && pos.argChild() == nil:
		// A non-empty token that matches no continuation and no argument slot
		// is a dead end, not something still being typed toward a match.
		if pos.Kind == KindRoot {
			res.Hint = "unknown command: " + partial
		} else {
			res.Hint = "unexpected: " + partial
		}
	case pos.Kind != KindRoot:
		res.Hint = formatUsage(e.root, pos, usageBudget)
	}
	return res
}

const usageBudget = 200

// splitCommand divides a command line into the already-typed tokens and the
// trailing partial token the cursor sits at the end of. A trailing space means
// the partial token is empty and a fresh token is starting.
func splitCommand(head string) (done []string, partial string) {
	fields := strings.Fields(head)
	if strings.HasSuffix(head, " ") || head == "" {
		return fields, ""
	}
	if len(fields) == 0 {
		return nil, ""
	}
	return fields[:len(fields)-1], fields[len(fields)-1]
}

// walk consumes the finished tokens from the root, following redirects, and
// returns the node whose children continue the command. ok is false when a
// token matched neither a literal child nor an argument slot; pos is then the
// node the walk stalled at.
func (e *Engine) walk(tokens []string) (pos *Node, ok bool) {
	pos = resolve(e.root, e.root)
	for _, tok := range tokens {
		if child := exactLiteral(pos, tok); child != nil {
			pos = resolve(e.root, child)
			continue
		}
		if arg := pos.argChild(); arg != nil {
			pos = resolve(e.root, arg)
			continue
		}
		return pos, false
	}
	return pos, true
}

func exactLiteral(n *Node, name string) *Node {
	if c := n.Children[name]; c != nil && c.Kind == KindLiteral {
		return c
	}
	return nil
}

func literalSuggestions(pos *Node, partial string) []Suggestion {
	lower := strings.ToLower(partial)
	var out []Suggestion
	for _, c := range pos.literalChildren() {
		if strings.HasPrefix(strings.ToLower(c.Name), lower) {
			out = append(out, Suggestion{Text: c.Name, Kind: SuggestLiteral})
		}
	}
	return out
}

func unknownHint(pos *Node, done []string) string {
	last := ""
	if len(done) > 0 {
		last = done[len(done)-1]
	}
	if pos.Kind == KindRoot {
		return "unknown command: " + last
	}
	return "unexpected: " + last
}
