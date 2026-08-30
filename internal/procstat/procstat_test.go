package procstat

import (
	"context"
	"os"
	"testing"
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
