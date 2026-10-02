package javadetect

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

func TestMajorOf(t *testing.T) {
	cases := map[string]int{
		"1.8.0_402": 8,
		"21":        21,
		"21.0.3":    21,
		"17.0.9":    17,
		"11.0.21":   11,
		"":          0,
		"garbage":   0,
	}
	for in, want := range cases {
		if got := majorOf(in); got != want {
			t.Errorf("majorOf(%q) = %d, want %d", in, got, want)
		}
	}
}

func TestFindPicksUpAShimAndReadsItsVersion(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell shim")
	}
	jvm := filepath.Join(t.TempDir(), "usr", "lib", "jvm")
	binDir := filepath.Join(jvm, "temurin-21", "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatal(err)
	}
	shim := "#!/bin/sh\necho 'openjdk version \"21.0.3\" 2024-04-16' 1>&2\n"
	javaPath := filepath.Join(binDir, "java")
	if err := os.WriteFile(javaPath, []byte(shim), 0o755); err != nil {
		t.Fatal(err)
	}
	if resolved, err := filepath.EvalSymlinks(javaPath); err == nil {
		javaPath = resolved
	}

	old := searchGlobs
	searchGlobs = []string{filepath.Join(jvm, "*", "bin", "java")}
	t.Cleanup(func() { searchGlobs = old })

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	got := Find(ctx)

	var hit *JDK
	for i := range got {
		if got[i].Path == javaPath {
			hit = &got[i]
		}
	}
	if hit == nil {
		t.Fatalf("Find did not return the shim at %s; got %+v", javaPath, got)
	}
	if hit.Major != 21 {
		t.Errorf("Major = %d, want 21", hit.Major)
	}
}

func TestFindNeverErrorsWithNothingInstalled(t *testing.T) {
	old := searchGlobs
	searchGlobs = []string{filepath.Join(t.TempDir(), "nope", "*", "java")}
	t.Cleanup(func() { searchGlobs = old })
	t.Setenv("JAVA_HOME", "")
	t.Setenv("PATH", t.TempDir())

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if got := Find(ctx); len(got) != 0 {
		t.Fatalf("want no JDKs, got %+v", got)
	}
}

func TestRequiredMajor(t *testing.T) {
	for v, want := range map[string]int{
		"1.12.2": 8, "1.16.5": 8, "1.17.1": 16, "1.18.2": 17, "1.20.1": 17,
		"1.20.4": 17, "1.20.5": 21, "1.21.1": 21, "26.1": 25, "26.2": 25, "": 0, "abc": 0,
	} {
		if got := RequiredMajor(v); got != want {
			t.Errorf("RequiredMajor(%q) = %d, want %d", v, got, want)
		}
	}
}

func TestPickTakesTheOldestJDKThatIsNewEnough(t *testing.T) {
	jdks := []JDK{{Path: "/a", Major: 26}, {Path: "/b", Major: 17}, {Path: "/c", Major: 21}}
	if got, ok := Pick(jdks, 18); !ok || got.Path != "/c" {
		t.Errorf("Pick(18) = %+v, %v; want /c", got, ok)
	}
	if _, ok := Pick(jdks, 27); ok {
		t.Error("Pick(27) found a JDK; none is new enough")
	}
}
