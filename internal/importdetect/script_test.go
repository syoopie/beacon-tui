package importdetect

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/syoopie/beacon-tui/internal/server"
)

func writeScript(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "run.sh")
	if err := os.WriteFile(path, []byte(body), 0o755); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	return path
}

func copyFixtureScript(t *testing.T, src string) string {
	t.Helper()
	data, err := os.ReadFile(src)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	path := filepath.Join(t.TempDir(), filepath.Base(src))
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	// 0770 rather than 0755 so a umask of 022 would narrow it, which is what
	// makes the mode assertions in TestApply load-bearing.
	if err := os.Chmod(path, scriptMode); err != nil {
		t.Fatalf("Chmod: %v", err)
	}
	return path
}

const scriptMode = 0o770

func TestInspectScript(t *testing.T) {
	tests := []struct {
		name  string
		body  string
		state server.ExecState
		line  int
		text  string
	}{
		{"exec java", "#!/bin/sh\nexec java -jar server.jar nogui\n", server.ExecOK, 2, "exec java -jar server.jar nogui"},
		{"bare java", "#!/bin/sh\njava -jar server.jar nogui\n", server.ExecMissing, 2, "java -jar server.jar nogui"},
		{"comments only", "#!/bin/sh\n# nothing to run\n\n", server.ExecMissing, 0, ""},
		{"empty file", "", server.ExecMissing, 0, ""},
		{"exec java home", "exec \"$JAVA_HOME/bin/java\" -jar x.jar\n", server.ExecOK, 1, "exec \"$JAVA_HOME/bin/java\" -jar x.jar"},
		{"wrapper script", "#!/bin/sh\n./launch.sh\n", server.ExecMissing, 2, "./launch.sh"},
		{"screen", "#!/bin/sh\nscreen -dmS mc java -jar server.jar\n", server.ExecMissing, 2, "screen -dmS mc java -jar server.jar"},
		{"execjava", "#!/bin/sh\nexecjava -jar server.jar\n", server.ExecMissing, 2, "execjava -jar server.jar"},
		{"bare exec", "#!/bin/sh\nexec\n", server.ExecMissing, 2, "exec"},
		{"exec without java", "#!/bin/sh\nexec ./launch.sh\n", server.ExecMissing, 2, "exec ./launch.sh"},
		{"uppercase java", "exec /opt/JAVA/bin/JAVA -jar x.jar\n", server.ExecOK, 1, "exec /opt/JAVA/bin/JAVA -jar x.jar"},
		{
			"trailing comments",
			"#!/bin/sh\n  java -jar server.jar nogui\n# a grandchild of the shell\n",
			server.ExecMissing,
			2,
			"  java -jar server.jar nogui",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := InspectScript(writeScript(t, tt.body))
			if err != nil {
				t.Fatalf("InspectScript: %v", err)
			}
			want := ExecCheck{State: tt.state, Line: tt.line, Text: tt.text}
			if got != want {
				t.Fatalf("InspectScript = %+v, want %+v", got, want)
			}
		})
	}
}

func TestInspectScriptMissingFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "run.sh")
	if _, err := InspectScript(path); err == nil || !strings.Contains(err.Error(), path) {
		t.Fatalf("InspectScript = %v, want an error naming %q", err, path)
	}
}

func TestPlanPatchAlreadyExecs(t *testing.T) {
	path := filepath.Join(fixtureRoot(t, "a"), "vanilla", "run.sh")

	p, ok, err := PlanPatch(path)
	if err != nil {
		t.Fatalf("PlanPatch: %v", err)
	}
	if ok {
		t.Fatalf("PlanPatch ok = true, want false")
	}
	if p != (Patch{}) {
		t.Fatalf("PlanPatch = %+v, want the zero Patch", p)
	}
}

func TestPlanPatchLegacyScript(t *testing.T) {
	path := filepath.Join(fixtureRoot(t, "a"), "legacy", "start.sh")

	p, ok, err := PlanPatch(path)
	if err != nil {
		t.Fatalf("PlanPatch: %v", err)
	}
	if !ok {
		t.Fatalf("PlanPatch ok = false, want true")
	}

	want := Patch{
		Path: path,
		Line: 3,
		Old:  "  java -jar server.jar nogui",
		New:  "  exec java -jar server.jar nogui",
	}
	if p != want {
		t.Fatalf("PlanPatch = %+v, want %+v", p, want)
	}

	wantDiff := path + ":3\n" +
		"-  java -jar server.jar nogui\n" +
		"+  exec java -jar server.jar nogui"
	if got := p.Diff(); got != wantDiff {
		t.Fatalf("Diff() = %q, want %q", got, wantDiff)
	}
	if strings.HasSuffix(p.Diff(), "\n") {
		t.Fatalf("Diff() = %q, want no trailing newline", p.Diff())
	}
}

func TestPlanPatchNoCommand(t *testing.T) {
	path := writeScript(t, "#!/bin/sh\n# nothing to run\n")

	if _, ok, err := PlanPatch(path); err == nil || !strings.Contains(err.Error(), path) {
		t.Fatalf("PlanPatch = %v (ok %v), want an error naming %q", err, ok, path)
	}
}

func TestApply(t *testing.T) {
	src := filepath.Join(fixtureRoot(t, "a"), "legacy", "start.sh")
	original, err := os.ReadFile(src)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	path := copyFixtureScript(t, src)

	p, ok, err := PlanPatch(path)
	if err != nil || !ok {
		t.Fatalf("PlanPatch = %v (ok %v), want a patch", err, ok)
	}
	if err := Apply(p); err != nil {
		t.Fatalf("Apply: %v", err)
	}

	check, err := InspectScript(path)
	if err != nil {
		t.Fatalf("InspectScript: %v", err)
	}
	want := ExecCheck{State: server.ExecOK, Line: 3, Text: "  exec java -jar server.jar nogui"}
	if check != want {
		t.Fatalf("InspectScript after Apply = %+v, want %+v", check, want)
	}

	patched, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if !bytes.HasSuffix(patched, []byte("\n")) {
		t.Fatalf("patched file = %q, want the trailing newline preserved", patched)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if info.Mode().Perm() != scriptMode {
		t.Fatalf("mode = %v, want %v", info.Mode().Perm(), os.FileMode(scriptMode))
	}

	backup, err := os.ReadFile(path + ".bak")
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if !bytes.Equal(backup, original) {
		t.Fatalf("backup = %q, want %q", backup, original)
	}
	backupInfo, err := os.Stat(path + ".bak")
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if backupInfo.Mode().Perm() != scriptMode {
		t.Fatalf("backup mode = %v, want %v", backupInfo.Mode().Perm(), os.FileMode(scriptMode))
	}

	if err := Apply(p); err == nil || !strings.Contains(err.Error(), path) {
		t.Fatalf("second Apply = %v, want an error naming %q", err, path)
	}
}

func TestApplyKeepsExistingBackup(t *testing.T) {
	path := copyFixtureScript(t, filepath.Join(fixtureRoot(t, "a"), "legacy", "start.sh"))
	sentinel := []byte("the operator's own backup\n")
	if err := os.WriteFile(path+".bak", sentinel, 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	p, ok, err := PlanPatch(path)
	if err != nil || !ok {
		t.Fatalf("PlanPatch = %v (ok %v), want a patch", err, ok)
	}
	if err := Apply(p); err != nil {
		t.Fatalf("Apply: %v", err)
	}

	backup, err := os.ReadFile(path + ".bak")
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if !bytes.Equal(backup, sentinel) {
		t.Fatalf("backup = %q, want %q", backup, sentinel)
	}

	check, err := InspectScript(path)
	if err != nil {
		t.Fatalf("InspectScript: %v", err)
	}
	if check.State != server.ExecOK {
		t.Fatalf("InspectScript after Apply = %+v, want ExecOK", check)
	}
}

func TestPlanPatchNonJavaCommand(t *testing.T) {
	path := writeScript(t, "#!/bin/sh\nexec ./launch.sh\n")

	_, ok, err := PlanPatch(path)
	if ok || err == nil {
		t.Fatalf("PlanPatch = ok %v, err %v; want a refusal for a non-java last command", ok, err)
	}
	if !strings.Contains(err.Error(), "java invocation") {
		t.Fatalf("error %q should explain the last command is not java", err)
	}
}
