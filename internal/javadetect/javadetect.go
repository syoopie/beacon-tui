// Package javadetect finds the JDKs installed on the host so Beacon can offer
// them as launch runtimes instead of making the operator type a path.
package javadetect

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
)

// JDK is one Java runtime found on the host.
type JDK struct {
	Path  string // absolute path to the java executable, symlinks resolved
	Major int    // feature version: 21, 17, 8; 0 when the version string did not parse
	Label string // "Java 21 (Homebrew)", for a picker
}

// searchGlobs are the java executables of every common install layout on macOS
// and Linux. A home directory is filled in by Find for the version managers.
var searchGlobs = []string{
	"/Library/Java/JavaVirtualMachines/*/Contents/Home/bin/java",
	"/opt/homebrew/opt/openjdk*/bin/java",
	"/opt/homebrew/opt/openjdk*/libexec/openjdk.jdk/Contents/Home/bin/java",
	"/opt/homebrew/Cellar/openjdk*/*/bin/java",
	"/opt/homebrew/Cellar/openjdk*/*/libexec/openjdk.jdk/Contents/Home/bin/java",
	"/usr/local/opt/openjdk*/bin/java",
	"/usr/local/Cellar/openjdk*/*/bin/java",
	"/usr/lib/jvm/*/bin/java",
	"/usr/lib/jvm/*/*/bin/java",
	"/usr/java/*/bin/java",
	"~/.sdkman/candidates/java/*/bin/java",
	"~/.asdf/installs/java/*/bin/java",
}

// Find returns the host's JDKs, newest feature version first, one per resolved
// path. It runs `java -version` on each candidate, so ctx should carry a
// timeout. It never errors: a path that is not there or a java that will not
// answer is left out.
func Find(ctx context.Context) []JDK {
	home, _ := os.UserHomeDir()

	candidates := map[string]struct{}{}
	add := func(p string) {
		if p == "" {
			return
		}
		if resolved, err := filepath.EvalSymlinks(p); err == nil {
			p = resolved
		}
		if filepath.IsAbs(p) {
			candidates[p] = struct{}{}
		}
	}

	for _, g := range searchGlobs {
		if strings.HasPrefix(g, "~/") {
			if home == "" {
				continue
			}
			g = filepath.Join(home, g[2:])
		}
		matches, _ := filepath.Glob(g)
		for _, m := range matches {
			add(m)
		}
	}
	if jh := os.Getenv("JAVA_HOME"); jh != "" {
		add(filepath.Join(jh, "bin", "java"))
	}
	if p, err := exec.LookPath("java"); err == nil {
		add(p)
	}

	found := probeAll(ctx, candidates)
	sort.Slice(found, func(i, j int) bool {
		if found[i].Major != found[j].Major {
			return found[i].Major > found[j].Major
		}
		return found[i].Path < found[j].Path
	})
	return found
}

func probeAll(ctx context.Context, paths map[string]struct{}) []JDK {
	const workers = 8
	jobs := make(chan string)
	results := make(chan JDK)

	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for p := range jobs {
				if jdk, ok := probe(ctx, p); ok {
					results <- jdk
				}
			}
		}()
	}
	go func() {
		for p := range paths {
			jobs <- p
		}
		close(jobs)
		wg.Wait()
		close(results)
	}()

	var out []JDK
	for jdk := range results {
		out = append(out, jdk)
	}
	return out
}

var versionRe = regexp.MustCompile(`version "([0-9][0-9._]*)"`)

func probe(ctx context.Context, path string) (JDK, bool) {
	info, err := os.Stat(path)
	if err != nil || info.IsDir() || info.Mode().Perm()&0o111 == 0 {
		return JDK{}, false
	}
	out, err := exec.CommandContext(ctx, path, "-version").CombinedOutput()
	if err != nil {
		return JDK{}, false
	}
	m := versionRe.FindSubmatch(out)
	if m == nil {
		return JDK{}, false
	}
	major := majorOf(string(m[1]))
	return JDK{Path: path, Major: major, Label: label(major, path)}, true
}

// majorOf turns a Java version string into its feature number: "1.8.0_402" is 8,
// "21.0.3" is 21, "17" is 17. Zero when nothing parses.
func majorOf(v string) int {
	parts := strings.FieldsFunc(v, func(r rune) bool { return r == '.' || r == '_' })
	if len(parts) == 0 {
		return 0
	}
	if parts[0] == "1" && len(parts) > 1 {
		n, _ := strconv.Atoi(parts[1])
		return n
	}
	n, _ := strconv.Atoi(parts[0])
	return n
}

func label(major int, path string) string {
	name := "Java"
	if major > 0 {
		name = "Java " + strconv.Itoa(major)
	}
	switch {
	case strings.Contains(path, "/homebrew/"), strings.Contains(path, "/Cellar/"):
		return name + " (Homebrew)"
	case strings.Contains(path, "/.sdkman/"):
		return name + " (SDKMAN)"
	case strings.Contains(path, "/.asdf/"):
		return name + " (asdf)"
	default:
		return name
	}
}
