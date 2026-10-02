package procstat

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestSampleThisProcess(t *testing.T) {
	got, err := Sample(context.Background(), os.Getpid(), "")
	if err != nil {
		t.Fatalf("Sample: %v", err)
	}
	if got.RSS <= 0 {
		t.Fatalf("RSS = %d, want a positive byte count", got.RSS)
	}
	if got.CPUPercent < 0 {
		t.Fatalf("CPUPercent = %f, want >= 0", got.CPUPercent)
	}
	if got.Uptime < 0 {
		t.Fatalf("Uptime = %s, want >= 0", got.Uptime)
	}
}

func TestParseETime(t *testing.T) {
	cases := map[string]time.Duration{
		"05:09":       5*time.Minute + 9*time.Second,
		"01:05:09":    time.Hour + 5*time.Minute + 9*time.Second,
		"2-03:04:05":  2*24*time.Hour + 3*time.Hour + 4*time.Minute + 5*time.Second,
		"11-22:33:44": 11*24*time.Hour + 22*time.Hour + 33*time.Minute + 44*time.Second,
		"49:28.46":    49*time.Minute + 28460*time.Millisecond, // macOS cputime
	}
	for in, want := range cases {
		got, err := parseETime(in)
		if err != nil || got != want {
			t.Fatalf("parseETime(%q) = %s %v, want %s", in, got, err, want)
		}
	}
	if _, err := parseETime("nonsense"); err == nil {
		t.Fatal("expected an error for a malformed etime")
	}
}

func TestSampleRejectsBadPID(t *testing.T) {
	if _, err := Sample(context.Background(), 0, ""); err == nil {
		t.Fatal("expected an error for pid 0")
	}
	// PID 2^31-1 is not a real process on the test machine.
	if _, err := Sample(context.Background(), 2147483647, ""); err == nil {
		t.Fatal("expected an error for a pid that does not exist")
	}
}

func TestMaxHeapReadsFlagsAndArgFiles(t *testing.T) {
	dir := t.TempDir()
	args := "# Xmx in here wins over the command line\n-Xmx8G -Xms4G\n# -Xmx99G\n"
	if err := os.WriteFile(filepath.Join(dir, "user_jvm_args.txt"), []byte(args), 0o644); err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		args []string
		want int64
	}{
		{[]string{"java", "-Xmx4096M", "-jar", "server.jar"}, 4 << 30},
		{[]string{"java", "-Xmx2g", "-XX:MaxHeapSize=3G"}, 3 << 30},
		{[]string{"java", "-Xmx1G", "@user_jvm_args.txt", "@libraries/missing.txt"}, 8 << 30},
		{[]string{"java", "-jar", "server.jar", "nogui"}, 0},
		{[]string{"java", "-Xmxlots"}, 0},
	}
	for _, c := range cases {
		if got := maxHeap(c.args, dir); got != c.want {
			t.Fatalf("maxHeap(%q) = %d, want %d", c.args, got, c.want)
		}
	}
}

func TestRate(t *testing.T) {
	t0 := time.Unix(1000, 0)
	prev := Stat{At: t0, CPUTime: 10 * time.Second, Uptime: time.Minute}
	cur := Stat{At: t0.Add(2 * time.Second), CPUTime: 13 * time.Second, Uptime: time.Minute + 2*time.Second}
	if got, ok := Rate(prev, cur); !ok || got != 150 {
		t.Fatalf("Rate = %f %v, want 150 true", got, ok)
	}
	restarted := Stat{At: t0.Add(3 * time.Second), CPUTime: time.Second, Uptime: time.Second}
	if _, ok := Rate(prev, restarted); ok {
		t.Fatal("a restarted process should not yield a rate")
	}
}
