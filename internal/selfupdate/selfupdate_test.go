package selfupdate

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestNewer(t *testing.T) {
	cases := []struct {
		latest, current string
		want            bool
	}{
		{"v0.2.0", "v0.1.0", true},
		{"v0.1.1", "v0.1.0", true},
		{"v1.0.0", "v0.9.9", true},
		{"v0.1.0", "v0.1.0", false},
		{"v0.1.0", "v0.2.0", false},
		{"v0.2.0", "dev", false},
		{"garbage", "v0.1.0", false},
		{"v0.2", "v0.1.0", true},  // semver reads v0.2 as v0.2.0
		{"1.0.0", "0.9.0", false}, // no leading v, rejected
	}
	for _, c := range cases {
		if got := newer(c.latest, c.current); got != c.want {
			t.Errorf("newer(%q, %q) = %v, want %v", c.latest, c.current, got, c.want)
		}
	}
}

func TestCheckReportsAvailable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/repos/acme/tool/releases/latest" {
			t.Errorf("unexpected path %q", r.URL.Path)
		}
		w.Write([]byte(`{"tag_name":"v1.4.0","name":"v1.4.0"}`))
	}))
	defer srv.Close()

	res, err := check(context.Background(), srv.URL, "acme/tool", "v1.3.0")
	if err != nil {
		t.Fatalf("check: %v", err)
	}
	if !res.Available || res.Latest != "v1.4.0" || res.Current != "v1.3.0" {
		t.Fatalf("result = %+v, want v1.4.0 available", res)
	}
}

func TestCheckNoUpdateWhenCurrent(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte(`{"tag_name":"v1.4.0"}`))
	}))
	defer srv.Close()

	res, err := check(context.Background(), srv.URL, "acme/tool", "v1.4.0")
	if err != nil {
		t.Fatalf("check: %v", err)
	}
	if res.Available {
		t.Fatalf("result = %+v, want no update", res)
	}
}

func TestCheckErrorsOnNon200(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "nope", http.StatusNotFound)
	}))
	defer srv.Close()

	if _, err := check(context.Background(), srv.URL, "acme/tool", "v1.0.0"); err == nil {
		t.Fatal("check returned nil error on a 404")
	}
}

func TestInstallCommand(t *testing.T) {
	got := InstallCommand("syoopie/beacon-tui")
	want := "curl -fsSL https://raw.githubusercontent.com/syoopie/beacon-tui/main/install.sh | bash"
	if got != want {
		t.Fatalf("InstallCommand = %q, want %q", got, want)
	}
}

// releaseServer serves one release asset and its .sha256 the way GitHub does.
func releaseServer(t *testing.T, bin []byte, sum string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/acme/tool/releases/download/v1.4.0/beacon_linux_arm64":
			w.Write(bin)
		case "/acme/tool/releases/download/v1.4.0/beacon_linux_arm64.sha256":
			w.Write([]byte(sum + "  beacon_linux_arm64\n"))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func oldBinary(t *testing.T) string {
	t.Helper()
	target := filepath.Join(t.TempDir(), "beacon")
	if err := os.WriteFile(target, []byte("old"), 0o755); err != nil {
		t.Fatal(err)
	}
	return target
}

func TestApplyReplacesTarget(t *testing.T) {
	bin := []byte("new beacon")
	h := sha256.Sum256(bin)
	srv := releaseServer(t, bin, hex.EncodeToString(h[:]))
	target := oldBinary(t)

	if err := apply(context.Background(), srv.URL, "acme/tool", "v1.4.0", target, "beacon_linux_arm64"); err != nil {
		t.Fatalf("apply: %v", err)
	}
	got, _ := os.ReadFile(target)
	if string(got) != "new beacon" {
		t.Fatalf("target holds %q, want the new binary", got)
	}
	if fi, _ := os.Stat(target); fi.Mode().Perm()&0o100 == 0 {
		t.Fatalf("target mode = %v, want executable", fi.Mode())
	}
}

func TestApplyRejectsChecksumMismatch(t *testing.T) {
	h := sha256.Sum256([]byte("something else"))
	srv := releaseServer(t, []byte("new beacon"), hex.EncodeToString(h[:]))
	target := oldBinary(t)

	if err := apply(context.Background(), srv.URL, "acme/tool", "v1.4.0", target, "beacon_linux_arm64"); err == nil {
		t.Fatal("apply accepted a binary that does not match its checksum")
	}
	if got, _ := os.ReadFile(target); string(got) != "old" {
		t.Fatalf("target holds %q after a failed update, want it untouched", got)
	}
}

func TestApplyMissingAsset(t *testing.T) {
	srv := releaseServer(t, nil, "")
	target := oldBinary(t)

	if err := apply(context.Background(), srv.URL, "acme/tool", "v1.4.0", target, "beacon_plan9_386"); err == nil {
		t.Fatal("apply succeeded with no asset for the platform")
	}
	if got, _ := os.ReadFile(target); string(got) != "old" {
		t.Fatalf("target holds %q after a failed update, want it untouched", got)
	}
}
