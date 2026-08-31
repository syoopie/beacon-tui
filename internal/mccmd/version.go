package mccmd

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// mcVersion is a parsed Minecraft release number: major.minor with an optional
// patch. "1.20" and "1.20.0" compare equal.
type mcVersion struct{ major, minor, patch int }

func parseVersion(s string) (mcVersion, error) {
	s = strings.TrimSpace(s)
	parts := strings.Split(s, ".")
	if len(parts) < 2 || len(parts) > 3 {
		return mcVersion{}, fmt.Errorf("mccmd: %q is not a Minecraft version", s)
	}
	var v mcVersion
	dst := []*int{&v.major, &v.minor, &v.patch}
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 {
			return mcVersion{}, fmt.Errorf("mccmd: %q is not a Minecraft version", s)
		}
		*dst[i] = n
	}
	return v, nil
}

func (v mcVersion) cmp(o mcVersion) int {
	switch {
	case v.major != o.major:
		return v.major - o.major
	case v.minor != o.minor:
		return v.minor - o.minor
	default:
		return v.patch - o.patch
	}
}

func (v mcVersion) sameMinor(o mcVersion) bool {
	return v.major == o.major && v.minor == o.minor
}

// versionMatch is what matchVersion decided.
type versionMatch struct {
	pick  string // embedded version to load; "" when none is usable
	exact bool   // pick equals the target
	minor bool   // pick shares the target's major.minor (patch drift only)
}

// matchVersion chooses the embedded tree for target: the newest embedded
// version that shares target's major.minor, else the newest embedded version
// no newer than target, else (target is older than everything embedded) none.
// An empty or unparseable target yields no pick.
func matchVersion(target string, embedded []string) versionMatch {
	want, err := parseVersion(target)
	if err != nil {
		return versionMatch{}
	}

	type cand struct {
		raw string
		v   mcVersion
	}
	var cands []cand
	for _, e := range embedded {
		v, err := parseVersion(e)
		if err != nil {
			continue
		}
		cands = append(cands, cand{e, v})
	}
	sort.Slice(cands, func(i, j int) bool { return cands[i].v.cmp(cands[j].v) < 0 })

	// cands is sorted ascending, so the last write to each of these wins.
	var sameMinorLE, sameMinorAny, notNewer *cand
	for i := range cands {
		c := &cands[i]
		if c.v.sameMinor(want) {
			sameMinorAny = c
			if c.v.cmp(want) <= 0 {
				sameMinorLE = c
			}
		}
		if c.v.cmp(want) <= 0 {
			notNewer = c
		}
	}

	switch {
	case sameMinorLE != nil:
		return versionMatch{pick: sameMinorLE.raw, exact: sameMinorLE.v.cmp(want) == 0, minor: true}
	case sameMinorAny != nil:
		return versionMatch{pick: sameMinorAny.raw, minor: true}
	case notNewer != nil:
		return versionMatch{pick: notNewer.raw}
	default:
		return versionMatch{}
	}
}
