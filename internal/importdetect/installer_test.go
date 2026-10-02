package importdetect

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/syoopie/beacon-tui/internal/server"
)

func TestInstallerPackLaunchesThroughBeacon(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "startserver.sh"), "#!/bin/sh\nwhile true; do java -jar x.jar; done\n")
	writeFile(t, filepath.Join(dir, "neoforge-21.1.251-installer.jar"), "")

	opts := LaunchOptions(dir)
	if len(opts) == 0 || opts[0].Script != "" || opts[0].Exec != server.ExecOK {
		t.Fatalf("LaunchOptions = %+v, want the installer launch first", opts)
	}
	cmd := opts[0].Command("nogui")
	for _, want := range []string{
		"neoforge-21.1.251-installer.jar --installServer",
		"exec java @user_jvm_args.txt @libraries/net/neoforged/neoforge/21.1.251/unix_args.txt",
		"' beacon nogui",
	} {
		if !strings.Contains(cmd, want) {
			t.Errorf("command %q lacks %q", cmd, want)
		}
	}
	if got := InstallerLabel(cmd); got != "NeoForge 21.1.251" {
		t.Errorf("InstallerLabel = %q, want NeoForge 21.1.251", got)
	}
	if v, loader := Identify(dir); v != "1.21.1" || loader != "neoforge" {
		t.Errorf("Identify = %q, %q; want 1.21.1, neoforge", v, loader)
	}
}

func TestForgeInstallerVersion(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "forge-1.20.1-47.4.20-installer.jar"), "")
	if v, loader := Identify(dir); v != "1.20.1" || loader != "forge" {
		t.Errorf("Identify = %q, %q; want 1.20.1, forge", v, loader)
	}
	if got := InstallerLabel(LaunchOptions(dir)[0].Base); got != "Forge 1.20.1-47.4.20" {
		t.Errorf("InstallerLabel = %q", got)
	}
}

func TestNeoForgeMCVersion(t *testing.T) {
	for v, want := range map[string]string{"21.1.251": "1.21.1", "20.4.237": "1.20.4", "21.0.10": "1.21", "26.1.0.5": "26.1"} {
		if got := (forgeInstall{loader: "neoforge", version: v}).mcVersion(); got != want {
			t.Errorf("mcVersion(%s) = %q, want %q", v, got, want)
		}
	}
}
