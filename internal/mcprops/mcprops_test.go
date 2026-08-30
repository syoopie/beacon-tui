package mcprops

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const sample = `#Minecraft server properties
#Sat Aug 30 12:00:00 UTC 2026
motd=A Minecraft Server
difficulty=easy
max-players=20
enable-rcon=false
rcon.password=
server-port=25565

# trailing comment
`

func writeProps(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "server.properties"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestGet(t *testing.T) {
	p, err := LoadProperties(writeProps(t, sample))
	if err != nil {
		t.Fatal(err)
	}
	if v, ok := p.Get("difficulty"); !ok || v != "easy" {
		t.Fatalf("difficulty = %q, %v", v, ok)
	}
	if v, ok := p.Get("rcon.password"); !ok || v != "" {
		t.Fatalf("empty value should still be present: %q, %v", v, ok)
	}
	if _, ok := p.Get("no-such-key"); ok {
		t.Fatal("absent key reported present")
	}
	if got := p.GetOr("pvp", "true"); got != "true" {
		t.Fatalf("GetOr default = %q", got)
	}
}

func TestSetRoundTripsUntouchedLines(t *testing.T) {
	dir := writeProps(t, sample)
	p, err := LoadProperties(dir)
	if err != nil {
		t.Fatal(err)
	}
	p.Set("difficulty", "hard")
	p.Set("enable-rcon", "true")
	p.Set("rcon.password", "s3cret")
	p.Set("white-list", "true") // new key, appended
	if err := p.Save(); err != nil {
		t.Fatal(err)
	}

	reloaded, err := LoadProperties(dir)
	if err != nil {
		t.Fatal(err)
	}
	for k, want := range map[string]string{
		"difficulty":    "hard",
		"enable-rcon":   "true",
		"rcon.password": "s3cret",
		"white-list":    "true",
		"motd":          "A Minecraft Server",
		"server-port":   "25565",
	} {
		if v, _ := reloaded.Get(k); v != want {
			t.Errorf("%s = %q, want %q", k, v, want)
		}
	}

	out, err := os.ReadFile(filepath.Join(dir, "server.properties"))
	if err != nil {
		t.Fatal(err)
	}
	for _, keep := range []string{"#Minecraft server properties", "# trailing comment"} {
		if !strings.Contains(string(out), keep) {
			t.Errorf("comment %q was dropped:\n%s", keep, out)
		}
	}
}

func TestLoadMissingFileThenSaveCreatesIt(t *testing.T) {
	dir := t.TempDir()
	p, err := LoadProperties(dir)
	if err != nil {
		t.Fatalf("missing file should not error: %v", err)
	}
	p.Set("server-port", "25580")
	if err := p.Save(); err != nil {
		t.Fatal(err)
	}
	reloaded, err := LoadProperties(dir)
	if err != nil {
		t.Fatal(err)
	}
	if v, _ := reloaded.Get("server-port"); v != "25580" {
		t.Fatalf("server-port = %q after create", v)
	}
}

func TestEULA(t *testing.T) {
	dir := t.TempDir()

	if ok, err := EULAAccepted(dir); err != nil || ok {
		t.Fatalf("missing eula.txt should read as not accepted: ok=%v err=%v", ok, err)
	}

	eula := filepath.Join(dir, "eula.txt")
	if err := os.WriteFile(eula, []byte("#By changing the setting below to TRUE you agree\neula=false\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if ok, _ := EULAAccepted(dir); ok {
		t.Fatal("eula=false read as accepted")
	}

	if err := AcceptEULA(dir); err != nil {
		t.Fatal(err)
	}
	if ok, err := EULAAccepted(dir); err != nil || !ok {
		t.Fatalf("after AcceptEULA: ok=%v err=%v", ok, err)
	}
	out, _ := os.ReadFile(eula)
	if !contains(string(out), "By changing the setting below") {
		t.Fatalf("AcceptEULA dropped the comment header:\n%s", out)
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || indexOf(s, sub) >= 0)
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
