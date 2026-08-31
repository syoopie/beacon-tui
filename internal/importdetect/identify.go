package importdetect

import (
	"archive/zip"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// Identify makes a best-effort guess at a server directory's Minecraft version
// and mod loader from the files a pack ships. Either result may be "" when
// nothing is conclusive; detection never fails and never runs Java. The values
// seed servers/<id>.toml, where the operator can correct them.
func Identify(dir string) (mcVersion, loader string) {
	loader = detectLoader(dir)
	mcVersion = detectVersion(dir, loader)
	return mcVersion, loader
}

var versionRe = regexp.MustCompile(`\b[0-9]+\.[0-9]+(?:\.[0-9]+)?\b`)

func cleanVersion(s string) string {
	// A loader artefact is often "1.20.1-47.4.20" or "server-1.20.1.jar"; take
	// the first bare version-looking token.
	return versionRe.FindString(s)
}

func detectLoader(dir string) string {
	switch {
	case isDir(filepath.Join(dir, "libraries", "net", "neoforged", "neoforge")):
		return "neoforge"
	case isDir(filepath.Join(dir, "libraries", "net", "minecraftforge", "forge")):
		return "forge"
	case isDir(filepath.Join(dir, "libraries", "net", "fabricmc", "fabric-loader")),
		exists(filepath.Join(dir, "fabric-server-launch.jar")),
		exists(filepath.Join(dir, "fabric-server-launcher.properties")),
		isDir(filepath.Join(dir, ".fabric")):
		return "fabric"
	case isDir(filepath.Join(dir, "libraries", "org", "quiltmc", "quilt-loader")):
		return "quilt"
	}
	if exists(filepath.Join(dir, "version_history.json")) {
		if v := paperFlavor(dir); v != "" {
			return v
		}
		return "paper"
	}
	for _, name := range serverJarNames(dir) {
		switch {
		case strings.HasPrefix(name, "paper"):
			return "paper"
		case strings.HasPrefix(name, "purpur"):
			return "purpur"
		case strings.HasPrefix(name, "folia"):
			return "folia"
		case strings.HasPrefix(name, "spigot"):
			return "spigot"
		case strings.HasPrefix(name, "craftbukkit"):
			return "craftbukkit"
		}
	}
	if jarWithVersionJSON(dir) != "" {
		return "vanilla"
	}
	return ""
}

// paperFlavor reads the fork name out of version_history.json's currentVersion
// string, e.g. "git-Purpur-2100 (MC: 1.20.1)".
func paperFlavor(dir string) string {
	data, err := os.ReadFile(filepath.Join(dir, "version_history.json"))
	if err != nil {
		return ""
	}
	lower := strings.ToLower(string(data))
	for _, fork := range []string{"purpur", "folia", "paper"} {
		if strings.Contains(lower, fork) {
			return fork
		}
	}
	return ""
}

func detectVersion(dir, loader string) string {
	// 1. A loader install unpacks the vanilla server under a version directory.
	// This is the most reliable source: the directory name is the MC version.
	// (The neoforge directory is deliberately not consulted here: its name is
	// the loader version, e.g. "21.1.73", not the Minecraft version.)
	for _, base := range []string{
		filepath.Join(dir, "libraries", "net", "minecraft", "server"),
		filepath.Join(dir, "libraries", "net", "minecraftforge", "forge"),
	} {
		for _, name := range subdirs(base) {
			if v := cleanVersion(name); v != "" {
				return v
			}
		}
	}

	// 2. A real Minecraft jar names its version in version.json.
	if jar := jarWithVersionJSON(dir); jar != "" {
		if v := versionFromJar(jar); v != "" {
			return v
		}
	}

	// 3. version_history.json: "... (MC: 1.20.1)".
	if data, err := os.ReadFile(filepath.Join(dir, "version_history.json")); err == nil {
		if m := regexp.MustCompile(`\(MC:\s*([0-9][0-9.]*)\)`).FindSubmatch(data); m != nil {
			return string(m[1])
		}
	}

	// 4. fabric-server-launcher.properties points at server-<v>.jar.
	if data, err := os.ReadFile(filepath.Join(dir, "fabric-server-launcher.properties")); err == nil {
		if v := cleanVersion(string(data)); v != "" {
			return v
		}
	}

	// 5. Last resort: a version-looking token in a server jar's file name.
	for _, name := range serverJarNames(dir) {
		if v := cleanVersion(name); v != "" {
			return v
		}
	}
	return ""
}

// jarWithVersionJSON returns the path to the first jar in dir that carries a
// top-level version.json, i.e. a genuine Minecraft server jar rather than a
// launcher shim.
func jarWithVersionJSON(dir string) string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return ""
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".jar") {
			continue
		}
		path := filepath.Join(dir, e.Name())
		if versionFromJar(path) != "" {
			return path
		}
	}
	return ""
}

func versionFromJar(path string) string {
	zr, err := zip.OpenReader(path)
	if err != nil {
		return ""
	}
	defer func() { _ = zr.Close() }()

	for _, f := range zr.File {
		if f.Name != "version.json" {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return ""
		}
		data, err := io.ReadAll(io.LimitReader(rc, 1<<16))
		_ = rc.Close()
		if err != nil {
			return ""
		}
		var v struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		}
		if json.Unmarshal(data, &v) != nil {
			return ""
		}
		if v.ID != "" {
			return cleanVersion(v.ID)
		}
		return cleanVersion(v.Name)
	}
	return ""
}

func serverJarNames(dir string) []string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var out []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".jar") {
			out = append(out, e.Name())
		}
	}
	return out
}

func subdirs(dir string) []string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var out []string
	for _, e := range entries {
		if e.IsDir() {
			out = append(out, e.Name())
		}
	}
	return out
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func isDir(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}
