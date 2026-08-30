package ui

import "regexp"

// logKind sorts a server log line into the console's two axes: which tab it
// belongs to, and how much it stands out inside the Server tab.
type logKind int

const (
	kindNormal  logKind = iota // shown plainly on the Server tab
	kindNoise                  // hidden when the Server tab is filtered, dimmed when it is not
	kindNotable                // highlighted on the Server tab, kept even when filtered
	kindChat                   // player activity; the Chat tab shows only these
)

type classRule struct {
	kind logKind
	re   *regexp.Regexp
}

// classRules is matched top to bottom, first hit wins. Known-spammy lines come
// first, even the ones logged at WARN, so they land in the Server tab's noise
// tier rather than being highlighted by the generic WARN rule below. The set is
// deliberately small: an unmatched line stays kindNormal, which is never wrong,
// only unhelpful.
var classRules = []classRule{
	{kindNoise, regexp.MustCompile(`]: Can't keep up!`)},
	{kindNoise, regexp.MustCompile(`moved (?:too quickly!|wrongly!)`)},
	{kindNoise, regexp.MustCompile(`]: Preparing (?:spawn area:|start region for)`)},
	{kindNoise, regexp.MustCompile(`]: Time elapsed:`)},
	{kindNoise, regexp.MustCompile(`/DEBUG\]`)},
	{kindNoise, regexp.MustCompile(`]: Loaded \d+ (?:advancements|recipes)`)},
	{kindNoise, regexp.MustCompile(`Mismatch in destroy block pos`)},

	{kindChat, regexp.MustCompile(`]: (?:\[Not Secure\] )?<[^<>]+> `)},
	{kindChat, regexp.MustCompile(`]: \[(?:Server|Not Secure)\] `)},
	{kindChat, regexp.MustCompile(`]: \* \S`)},
	{kindChat, regexp.MustCompile(` (?:joined|left) the game\b`)},
	{kindChat, regexp.MustCompile(` has (?:made the advancement|completed the challenge|reached the goal) `)},

	{kindNotable, regexp.MustCompile(`/(?:WARN|ERROR|FATAL)\]`)},
	{kindNotable, regexp.MustCompile(`]: Done \(`)},
	{kindNotable, regexp.MustCompile(`]: (?:Starting minecraft server|Stopping server|Stopping the server)`)},
	{kindNotable, regexp.MustCompile(`]: You need to agree to the EULA`)},
}

func classify(line string) logKind {
	for _, r := range classRules {
		if r.re.MatchString(line) {
			return r.kind
		}
	}
	return kindNormal
}
