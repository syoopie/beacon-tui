package selfupdate

import (
	"context"
	"net/http"
	"net/http/httptest"
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
		{"v0.2", "v0.1.0", false},
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

func TestUpdateCommand(t *testing.T) {
	got := UpdateCommand("syoopie/beacon-tui")
	want := "curl -fsSL https://raw.githubusercontent.com/syoopie/beacon-tui/main/install.sh | bash"
	if got != want {
		t.Fatalf("UpdateCommand = %q, want %q", got, want)
	}
}
