package importdetect

import (
	"archive/zip"
	"os"
	"path/filepath"
	"testing"
)

func mkdirs(t *testing.T, dir string, rel ...string) {
	t.Helper()
	for _, r := range rel {
		if err := os.MkdirAll(filepath.Join(dir, r), 0o755); err != nil {
			t.Fatal(err)
		}
	}
}

func writeFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

// writeJar writes a jar (zip) containing the given name->content entries.
func writeJar(t *testing.T, path string, entries map[string]string) {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	zw := zip.NewWriter(f)
	for name, content := range entries {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestIdentifyForge(t *testing.T) {
	dir := t.TempDir()
	mkdirs(t, dir,
		filepath.Join("libraries", "net", "minecraftforge", "forge", "1.20.1-47.4.20"),
		filepath.Join("libraries", "net", "minecraft", "server", "1.20.1"),
	)
	writeFile(t, filepath.Join(dir, "run.sh"), "#!/bin/sh\n")

	v, loader := Identify(dir)
	if v != "1.20.1" || loader != "forge" {
		t.Fatalf("Identify = %q, %q; want 1.20.1, forge", v, loader)
	}
}

func TestIdentifyNeoForge(t *testing.T) {
	dir := t.TempDir()
	mkdirs(t, dir, filepath.Join("libraries", "net", "neoforged", "neoforge", "21.1.73"))
	writeFile(t, filepath.Join(dir, "libraries", "net", "minecraft", "server", "1.21.1", "keep"), "")

	v, loader := Identify(dir)
	// The neoforge dir names its own version (21.x); the MC version comes from
	// the minecraft/server directory.
	if v != "1.21.1" || loader != "neoforge" {
		t.Fatalf("Identify = %q, %q; want 1.21.1, neoforge", v, loader)
	}
}

func TestIdentifyFabric(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "fabric-server-launcher.properties"), "serverJar=server-1.20.4.jar\n")

	v, loader := Identify(dir)
	if v != "1.20.4" || loader != "fabric" {
		t.Fatalf("Identify = %q, %q; want 1.20.4, fabric", v, loader)
	}
}

func TestIdentifyPaperFromVersionHistory(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "version_history.json"),
		`{"currentVersion":"git-Paper-196 (MC: 1.20.4)"}`)

	v, loader := Identify(dir)
	if v != "1.20.4" || loader != "paper" {
		t.Fatalf("Identify = %q, %q; want 1.20.4, paper", v, loader)
	}
}

func TestIdentifyPurpurFlavor(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "version_history.json"),
		`{"currentVersion":"git-Purpur-2100 (MC: 1.20.4)"}`)

	if _, loader := Identify(dir); loader != "purpur" {
		t.Fatalf("loader = %q, want purpur", loader)
	}
}

func TestIdentifyVanillaFromJar(t *testing.T) {
	dir := t.TempDir()
	writeJar(t, filepath.Join(dir, "server.jar"), map[string]string{
		"version.json": `{"id":"1.20.2","name":"1.20.2"}`,
		"pack.mcmeta":  "{}",
	})

	v, loader := Identify(dir)
	if v != "1.20.2" || loader != "vanilla" {
		t.Fatalf("Identify = %q, %q; want 1.20.2, vanilla", v, loader)
	}
}

func TestIdentifyShimJarIsNotVanilla(t *testing.T) {
	dir := t.TempDir()
	// A launcher shim jar with no version.json must not be read as a real
	// Minecraft server jar.
	writeJar(t, filepath.Join(dir, "server.jar"), map[string]string{
		"net/neoforged/serverstarterjar/Main.class": "\xca\xfe\xba\xbe",
	})
	writeFile(t, filepath.Join(dir, "run.sh"), "#!/bin/sh\n")

	v, loader := Identify(dir)
	if v != "" || loader != "" {
		t.Fatalf("Identify = %q, %q; want empty for a bare shim", v, loader)
	}
}

func TestIdentifyUnknown(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "run.sh"), "#!/bin/sh\n")

	v, loader := Identify(dir)
	if v != "" || loader != "" {
		t.Fatalf("Identify = %q, %q; want empty, empty", v, loader)
	}
}
