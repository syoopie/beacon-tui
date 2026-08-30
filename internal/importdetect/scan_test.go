package importdetect

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/syoopie/beacon-tui/internal/server"
)

func fixtureRoot(t *testing.T, name string) string {
	t.Helper()
	dir, err := filepath.Abs(filepath.Join("..", "..", "testdata", "import", "roots", name))
	if err != nil {
		t.Fatalf("Abs: %v", err)
	}
	return dir
}

func TestScanFixtureRoots(t *testing.T) {
	a, b := fixtureRoot(t, "a"), fixtureRoot(t, "b")

	got, err := Scan([]string{a, b})
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}

	want := []Candidate{
		{
			Dir:    filepath.Join(a, "legacy"),
			Base:   "legacy",
			Start:  "./start.sh nogui",
			Script: "start.sh",
			Exec:   server.ExecMissing,
			Port:   25565,
		},
		{
			Dir:   filepath.Join(a, "paper"),
			Base:  "paper",
			Start: "java -jar paper-1.20.4-496.jar nogui",
			Exec:  server.ExecOK,
			Port:  25565,
		},
		{
			Dir:    filepath.Join(a, "survival"),
			Base:   "survival",
			Start:  "./run.sh nogui",
			Script: "run.sh",
			Exec:   server.ExecOK,
			Port:   25565,
		},
		{
			Dir:    filepath.Join(a, "vanilla"),
			Base:   "vanilla",
			Start:  "./run.sh nogui",
			Script: "run.sh",
			Exec:   server.ExecOK,
			Port:   25566,
		},
		{
			Dir:    filepath.Join(b, "survival"),
			Base:   "survival",
			Start:  "./run.sh nogui",
			Script: "run.sh",
			Exec:   server.ExecOK,
			Port:   25565,
		},
	}

	if len(got) != len(want) {
		t.Fatalf("Scan returned %d candidates, want %d: %+v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("candidate %d = %+v, want %+v", i, got[i], want[i])
		}
	}
}

func TestScanErrors(t *testing.T) {
	dir := t.TempDir()
	missing := filepath.Join(dir, "nope")
	file := filepath.Join(dir, "notes.txt")
	if err := os.WriteFile(file, []byte("hello\n"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	tests := []struct {
		name  string
		roots []string
		names string
	}{
		{"no roots", []string{}, "scan root"},
		{"missing root", []string{missing}, missing},
		{"root is a regular file", []string{file}, file},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Scan(tt.roots)
			if err == nil {
				t.Fatalf("Scan = %+v, want an error", got)
			}
			if !strings.Contains(err.Error(), tt.names) {
				t.Fatalf("error %q does not name %q", err, tt.names)
			}
		})
	}
}

func TestScanRootIsItselfAServer(t *testing.T) {
	dir := filepath.Join(fixtureRoot(t, "a"), "vanilla")

	got, err := Scan([]string{dir})
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}

	want := Candidate{
		Dir:    dir,
		Base:   "vanilla",
		Start:  "./run.sh nogui",
		Script: "run.sh",
		Exec:   server.ExecOK,
		Port:   25566,
	}
	if len(got) != 1 || got[0] != want {
		t.Fatalf("Scan = %+v, want exactly %+v", got, want)
	}
}

func TestScanServerRootDoesNotDescendIntoPluginsFolder(t *testing.T) {
	root := t.TempDir()
	server := filepath.Join(root, "survival")
	plugins := filepath.Join(server, "plugins")
	if err := os.MkdirAll(plugins, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(server, "run.sh"), []byte("#!/bin/sh\nexec java -jar paper.jar\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	// A Paper plugin named like a server jar.
	if err := os.WriteFile(filepath.Join(plugins, "paper-economy.jar"), nil, 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := Scan([]string{server})
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	if len(got) != 1 || got[0].Dir != server {
		t.Fatalf("Scan = %+v, want exactly the server directory", got)
	}
}

func TestScanOverlappingRoots(t *testing.T) {
	a := fixtureRoot(t, "a")
	vanilla := filepath.Join(a, "vanilla")

	got, err := Scan([]string{a, vanilla})
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}

	seen := make(map[string]int)
	for _, c := range got {
		seen[c.Dir]++
	}
	if seen[vanilla] != 1 {
		t.Fatalf("%s appears %d times in %+v, want 1", vanilla, seen[vanilla], got)
	}
	if len(got) != len(seen) {
		t.Fatalf("Scan returned %d candidates over %d directories", len(got), len(seen))
	}
}

func TestScanJarPrecedence(t *testing.T) {
	tests := []struct {
		name  string
		jars  []string
		start string
	}{
		{
			"server.jar wins",
			[]string{"server.jar", "paper-1.20.4-496.jar", "fabric-server-launch.jar"},
			"java -jar server.jar nogui",
		},
		{
			"paper beats fabric",
			[]string{"paper-1.20.4-496.jar", "fabric-server-launch.jar"},
			"java -jar paper-1.20.4-496.jar nogui",
		},
		{
			"sorted first paper wins",
			[]string{"paper-1.20.4-496.jar", "paper-1.20.4-40.jar"},
			"java -jar paper-1.20.4-40.jar nogui",
		},
		{
			"fabric alone",
			[]string{"fabric-server-launch.jar"},
			"java -jar fabric-server-launch.jar nogui",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			dir := filepath.Join(root, "srv")
			if err := os.Mkdir(dir, 0o755); err != nil {
				t.Fatalf("Mkdir: %v", err)
			}
			for _, jar := range tt.jars {
				if err := os.WriteFile(filepath.Join(dir, jar), nil, 0o644); err != nil {
					t.Fatalf("WriteFile: %v", err)
				}
			}

			got, err := Scan([]string{root})
			if err != nil {
				t.Fatalf("Scan: %v", err)
			}

			want := Candidate{Dir: dir, Base: "srv", Start: tt.start, Exec: server.ExecOK, Port: 25565}
			if len(got) != 1 || got[0] != want {
				t.Fatalf("Scan = %+v, want exactly %+v", got, want)
			}
		})
	}
}

func TestLaunchOptionsListsEveryMethodScriptsFirst(t *testing.T) {
	dir := t.TempDir()
	write := func(name, body string) {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o755); err != nil {
			t.Fatalf("WriteFile %s: %v", name, err)
		}
	}
	write("run.sh", "#!/bin/sh\nexec java -jar server.jar \"$@\"\n")
	write("start.sh", "#!/bin/sh\njava -jar server.jar nogui\n")
	write("server.jar", "")

	got := LaunchOptions(dir)
	want := []LaunchOption{
		{Label: "run.sh", Script: "run.sh", Base: "./run.sh", Exec: server.ExecOK},
		{Label: "start.sh", Script: "start.sh", Base: "./start.sh", Exec: server.ExecMissing},
		{Label: "server.jar", Base: "java -jar server.jar", Exec: server.ExecOK},
	}
	if len(got) != len(want) {
		t.Fatalf("LaunchOptions = %+v, want %+v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("option %d = %+v, want %+v", i, got[i], want[i])
		}
	}

	if cmd := got[0].Command(""); cmd != "./run.sh" {
		t.Errorf("Command(\"\") = %q, want ./run.sh", cmd)
	}
	if cmd := got[2].Command("nogui"); cmd != "java -jar server.jar nogui" {
		t.Errorf("Command(\"nogui\") = %q, want java -jar server.jar nogui", cmd)
	}
}
