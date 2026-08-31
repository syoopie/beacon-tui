package mccmd

import (
	"bytes"
	"compress/gzip"
	"embed"
	"fmt"
	"io/fs"
	"sort"
	"strings"
	"sync"
)

//go:embed trees/*.json.gz
var treeFS embed.FS

// bundledSource serves a vanilla command tree from the trees vendored into the
// binary. It is always present and needs no configuration; its only input is
// the server's Minecraft version.
type bundledSource struct{}

// Bundled is the always-on vanilla command vocabulary.
func Bundled() VocabularySource { return bundledSource{} }

func (bundledSource) Name() string  { return "bundled" }
func (bundledSource) Priority() int { return 0 }

func (bundledSource) Tree(ctx Context) (*Node, error) {
	m := matchVersion(ctx.MCVersion, embeddedVersions())
	if m.pick == "" {
		return nil, nil
	}
	return loadEmbedded(m.pick)
}

var (
	versionsOnce sync.Once
	versionsList []string
)

// embeddedVersions is the sorted list of Minecraft versions with a vendored
// tree, derived from the file names under trees/.
func embeddedVersions() []string {
	versionsOnce.Do(func() {
		entries, err := fs.ReadDir(treeFS, "trees")
		if err != nil {
			return
		}
		for _, e := range entries {
			name := strings.TrimSuffix(e.Name(), ".json.gz")
			if name != e.Name() {
				versionsList = append(versionsList, name)
			}
		}
		sort.Strings(versionsList)
	})
	return versionsList
}

func loadEmbedded(version string) (*Node, error) {
	data, err := treeFS.ReadFile("trees/" + version + ".json.gz")
	if err != nil {
		return nil, fmt.Errorf("mccmd: opening bundled tree %s: %w", version, err)
	}
	gz, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("mccmd: reading bundled tree %s: %w", version, err)
	}
	defer func() { _ = gz.Close() }()

	root, err := Parse(gz)
	if err != nil {
		return nil, fmt.Errorf("mccmd: bundled tree %s: %w", version, err)
	}
	return root, nil
}
