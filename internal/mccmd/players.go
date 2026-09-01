package mccmd

import (
	"sort"
	"strings"
)

// playerParsers are the argument parsers whose value is a player or a selector
// that resolves to players. When the console has a live roster from RCON, the
// names of the people online are the completions worth offering for these
// slots, the way the vanilla client fills them in.
var playerParsers = map[string]bool{
	"minecraft:entity":       true, // /kill <targets>, /tp, /effect, …
	"minecraft:game_profile": true, // /whitelist, /op, /ban, …
	"minecraft:score_holder": true, // /scoreboard players
}

// SetPlayers hands the engine the current online roster, from the console's
// RCON player poll. It is kept apart from the command tree on purpose: the
// roster changes as people join and leave, far more often than the tree, and
// rebuilding the tree for it would also reload the recall history. Safe to call
// with the same list repeatedly; the console does on every poll.
func (e *Engine) SetPlayers(names []string) {
	seen := make(map[string]bool, len(names))
	out := make([]string, 0, len(names))
	for _, n := range names {
		if n = strings.TrimSpace(n); n != "" && !seen[n] {
			seen[n] = true
			out = append(out, n)
		}
	}
	sort.Strings(out)
	e.players = out
}

// playerSuggestions offers the online players whose name has the typed prefix,
// for an argument slot that takes a player. Empty unless a roster has been set
// and arg is a player-valued parser.
func (e *Engine) playerSuggestions(arg *Node, partial string) []Suggestion {
	if len(e.players) == 0 || arg == nil || !playerParsers[arg.Parser] {
		return nil
	}
	lower := strings.ToLower(partial)
	var out []Suggestion
	for _, name := range e.players {
		if strings.HasPrefix(strings.ToLower(name), lower) {
			out = append(out, Suggestion{Text: name, Kind: SuggestArgument, Detail: "online"})
		}
	}
	return out
}
