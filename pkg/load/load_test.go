package load

import (
	"context"
	"testing"
	"time"
)

func TestRun_CPULoadAndMemLoad_DoNotError(t *testing.T) {
	t.Helper()
	ctx := context.Background()

	tests := []Config{
		{
			Mode:        ModeCPU,
			Duration:    100 * time.Millisecond,
			Parallelism: 2,
		},
		{
			Mode:     ModeMem,
			Duration: 100 * time.Millisecond,
			AllocMB:  10,
		},
		{
			Mode:        ModeCPUMem,
			Duration:    100 * time.Millisecond,
			Parallelism: 2,
			AllocMB:     10,
		},
	}

	for _, cfg := range tests {
		if err := Run(ctx, cfg); err != nil {
			t.Fatalf("Run(%+v) returned error: %v", cfg, err)
		}
	}
}

func TestRun_ExceedsDurationLimitReturnsError(t *testing.T) {
	ctx := context.Background()

	cfg := Config{
		Mode:        ModeCPU,
		Duration:    DefaultLimits.MaxDuration + time.Second,
		Parallelism: 1,
	}

	if err := Run(ctx, cfg); err == nil {
		t.Fatalf("expected error for duration > MaxDuration, but got nil")
	}
}
