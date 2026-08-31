package mccmd

// Completer is the console front end's whole view of this package.
type Completer interface {
	// Complete answers "given this console input with the cursor here, what can
	// be typed next, and what does the current command take". It is pure and
	// cheap; call it on every keystroke.
	Complete(line string, cursor int) Result
}

// Result is one completion query's answer.
type Result struct {
	Suggestions []Suggestion // ranked, already filtered to the partial token
	Replace     Span         // the region of line a chosen suggestion overwrites
	Hint        string       // "<rule> [<value>]", or "unknown command: gamrule"; "" when there is nothing to say
	Degraded    string       // mirrors [Engine.Degraded]; the console shows it as a dim note
}

// Span is a half-open range of byte offsets into a command line.
type Span struct{ Start, End int }

// SuggestionKind tags where a suggestion came from, so the UI can style or
// annotate it.
type SuggestionKind uint8

const (
	SuggestLiteral  SuggestionKind = iota // a subcommand or enum literal from the tree
	SuggestArgument                       // a value for the argument under the cursor (a later phase: player names)
	SuggestHistory                        // a past console line (fallback when the tree is silent)
)

// Suggestion is one completion candidate.
type Suggestion struct {
	Text   string
	Kind   SuggestionKind
	Detail string // optional right-column note, e.g. the argument type
}

// verify at compile time that the engine satisfies the front end's contract.
var _ Completer = (*Engine)(nil)
