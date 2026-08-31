package procstat

import (
	"context"
	"os"
	"testing"
	"time"
)

func TestSampleThisProcess(t *testing.T) {
	got, err := Sample(context.Background(), os.Getpid())
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
	if _, err := Sample(context.Background(), 0); err == nil {
		t.Fatal("expected an error for pid 0")
	}
	// PID 2^31-1 is not a real process on the test machine.
	if _, err := Sample(context.Background(), 2147483647); err == nil {
		t.Fatal("expected an error for a pid that does not exist")
	}
}
