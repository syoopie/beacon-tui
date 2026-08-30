// Package selfupdate asks GitHub whether a newer tagged release exists. It only
// reports; it never downloads or replaces the binary. The install script does
// that.
package selfupdate

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"golang.org/x/mod/semver"
)

// Result is the outcome of a check.
type Result struct {
	Current   string // the running version, verbatim
	Latest    string // newest release tag, e.g. "v0.2.0"
	Available bool   // Latest is a well-formed version strictly newer than Current
}

// UpdateCommand is the one-liner that installs the newest release for a repo.
func UpdateCommand(repo string) string {
	return "curl -fsSL https://raw.githubusercontent.com/" + repo + "/main/install.sh | bash"
}

const apiBase = "https://api.github.com"

// Check reports whether repo has a release newer than current. A network error,
// a missing release, or an unparseable current version yields a zero Result and,
// for the network case, an error; callers should treat any failure as "no news".
func Check(ctx context.Context, repo, current string) (Result, error) {
	return check(ctx, apiBase, repo, current)
}

func check(ctx context.Context, base, repo, current string) (Result, error) {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	url := fmt.Sprintf("%s/repos/%s/releases/latest", base, repo)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return Result{Current: current}, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "beacon-selfupdate")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return Result{Current: current}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return Result{Current: current}, fmt.Errorf("github releases: %s", resp.Status)
	}

	var body struct {
		TagName string `json:"tag_name"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return Result{Current: current}, err
	}

	return Result{
		Current:   current,
		Latest:    body.TagName,
		Available: newer(body.TagName, current),
	}, nil
}

// newer reports whether latest is a strictly higher semantic version than
// current. Anything golang.org/x/mod/semver rejects on either side is not newer,
// so a "dev" build is never nagged.
func newer(latest, current string) bool {
	latest, current = strings.TrimSpace(latest), strings.TrimSpace(current)
	if !semver.IsValid(latest) || !semver.IsValid(current) {
		return false
	}
	return semver.Compare(latest, current) > 0
}
