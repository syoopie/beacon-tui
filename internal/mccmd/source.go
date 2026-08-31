package mccmd

// Context is what a source needs to decide which tree to return.
type Context struct {
	MCVersion string // "1.20.1"; "" when detection failed or the field is unset
	Loader    string // "vanilla" | "forge" | "fabric" | "paper" | ""; unused until the RCON phase
	ServerID  string
}

// VocabularySource contributes a command tree for one server.
//
// Tree is called synchronously each time the [Engine] is built, so an
// implementation must return promptly. A source backed by a slow probe (a later
// phase reads a running server's /help over RCON) is expected to cache its last
// parsed result and be fed fresh input out of band, the same way the UI model
// already receives RCON player snapshots; Tree then just hands back the cache.
// Returning (nil, nil) means the source has nothing to say for this server.
type VocabularySource interface {
	Name() string
	Priority() int // higher wins conflicts when trees are merged
	Tree(Context) (*Node, error)
}
