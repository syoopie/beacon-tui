// Package mccmd answers, for the beacon console, "what can be typed next and
// what does this command take". Command vocabulary is a tree that mirrors
// Mojang's generated commands.json; one or more [VocabularySource] values feed
// it and the [Engine] merges them. The console front end depends only on
// [Completer].
//
// Typed-command recall is a separate concern: see [History]. It is not a
// vocabulary source, because a past line is recall, not grammar.
package mccmd
