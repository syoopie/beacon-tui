// Package selfupdate asks GitHub whether a newer tagged release exists, and
// replaces the running binary with one.
package selfupdate

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"runtime"
	"strings"
	"time"

	"github.com/minio/selfupdate"
	"golang.org/x/mod/semver"
)

// Result is the outcome of a check.
type Result struct {
	Current   string // the running version, verbatim
	Latest    string // newest release tag, e.g. "v0.2.0"
	Available bool   // Latest is a well-formed version strictly newer than Current
}

// UpdateCommand is what a user runs to move to the newest release.
const UpdateCommand = "beacon update"

// InstallCommand is the one-liner that installs the newest release for a repo.
func InstallCommand(repo string) string {
	return "curl -fsSL https://raw.githubusercontent.com/" + repo + "/main/install.sh | bash"
}

const (
	apiBase      = "https://api.github.com"
	downloadBase = "https://github.com"
)

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

// Apply downloads tag's binary for this OS and architecture, checks it against
// the SHA-256 the release publishes beside it, and swaps it in for the file at
// target. The swap is a rename, so a running beacon keeps its old binary and a
// failed update leaves target as it was.
func Apply(ctx context.Context, repo, tag, target string) error {
	return apply(ctx, downloadBase, repo, tag, target, AssetName(runtime.GOOS, runtime.GOARCH))
}

// AssetName is the release file for one platform, as the release workflow and
// install.sh name it.
func AssetName(goos, goarch string) string {
	return "beacon_" + goos + "_" + goarch
}

func apply(ctx context.Context, base, repo, tag, target, asset string) error {
	opts := selfupdate.Options{TargetPath: target}
	if err := opts.CheckPermissions(); err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()

	url := fmt.Sprintf("%s/%s/releases/download/%s/%s", base, repo, tag, asset)
	sum, err := fetch(ctx, url+".sha256")
	if err != nil {
		return err
	}
	fields := strings.Fields(string(sum))
	if len(fields) == 0 {
		return fmt.Errorf("%s.sha256 is empty", asset)
	}
	if opts.Checksum, err = hex.DecodeString(fields[0]); err != nil || len(opts.Checksum) != 32 {
		return fmt.Errorf("%s.sha256 does not hold a SHA-256", asset)
	}

	bin, err := fetch(ctx, url)
	if err != nil {
		return err
	}
	if err := selfupdate.Apply(bytes.NewReader(bin), opts); err != nil {
		if rerr := selfupdate.RollbackError(err); rerr != nil {
			return fmt.Errorf("%w; restoring the old binary also failed: %v", err, rerr)
		}
		return err
	}
	return nil
}

func fetch(ctx context.Context, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "beacon-selfupdate")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, errors.New("download " + url + ": " + resp.Status)
	}
	return io.ReadAll(resp.Body)
}
