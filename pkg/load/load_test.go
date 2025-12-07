package load

import (
	"context"
	"errors"
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

type fixedRand struct {
	v float64
}

func (f fixedRand) Float64() float64 {
	return f.v
}

// I/Oモードが正常に動作し、エラーを返さないことの確認
func TestRun_IOLoad_DoesNotError(t *testing.T) {
	t.Helper()
	ctx := context.Background()

	cfg := Config{
		Mode:     ModeIO,
		Duration: 50 * time.Millisecond,
		IOBytes:  4 * 1024,
	}

	if err := Run(ctx, cfg); err != nil {
		t.Fatalf("Run(%+v) returned error: %v", cfg, err)
	}
}

// error_rate=1.0の時、必ずErrInjectedが返ることの確認
func TestRun_ErrorRateAlwaysOne_ReturnsInjectedError(t *testing.T) {
	ctx := context.Background()

	cfg := Config{
		Mode:        ModeCPU,
		Duration:    50 * time.Millisecond,
		Parallelism: 1,
		ErrorRate:   1.0,
	}

	err := Run(ctx, cfg)
	if !errors.Is(err, ErrInjected) {
		t.Fatalf("expected ErrInjected, got %v", err)
	}
}

// 中間の error_rate の時、Randによってエラー/非エラーが切り替わることを確認
func TestRun_ErrorRateIntermediate_UsesRand(t *testing.T) {
	t.Helper()
	ctx := context.Background()

	t.Run("no error when rand >= rate", func(t *testing.T) {
		cfg := Config{
			Mode:        ModeCPU,
			Duration:    50 * time.Millisecond,
			Parallelism: 1,
			ErrorRate:   0.5,
			Rand:        fixedRand{v: 0.75}, // 0.75 >= 0.5 →エラーなし
		}

		if err := Run(ctx, cfg); err != nil {
			t.Fatalf("Run returned unexpected error: %v", err)
		}
	})

	t.Run("error when rand < rate", func(t *testing.T) {
		cfg := Config{
			Mode:        ModeCPU,
			Duration:    50 * time.Millisecond,
			Parallelism: 1,
			ErrorRate:   0.5,
			Rand:        fixedRand{v: 0.25}, // 0.25 < 0.5 → ErrInjected
		}

		err := Run(ctx, cfg)
		if !errors.Is(err, ErrInjected) {
			t.Fatalf("expected ErrInjected, got %v", err)
		}
	})
}

// Latency,ErrorRateのバリデーションとModeIOの必須パラメータを確認
func TestValidateConfig_InvalidLatencyAndErrorRateAndIOMode(t *testing.T) {
	t.Run("negative latency is invalid", func(t *testing.T) {
		cfg := Config{
			Mode:        ModeCPU,
			Duration:    time.Second,
			Parallelism: 1,
			Latency:     -1 * time.Millisecond,
		}
		if err := validateConfig(cfg, DefaultLimits); err == nil {
			t.Fatalf("expected error for negative latency, got nil")
		}
	})

	t.Run("error_rate < 0 is invalid", func(t *testing.T) {
		cfg := Config{
			Mode:        ModeCPU,
			Duration:    time.Second,
			Parallelism: 1,
			ErrorRate:   -0.1,
		}
		if err := validateConfig(cfg, DefaultLimits); err == nil {
			t.Fatalf("expected error for error_rate < 0, got nil")
		}
	})

	t.Run("error_rate > 1 is invalid", func(t *testing.T) {
		cfg := Config{
			Mode:        ModeCPU,
			Duration:    time.Second,
			Parallelism: 1,
			ErrorRate:   1.1,
		}
		if err := validateConfig(cfg, DefaultLimits); err == nil {
			t.Fatalf("expected error for error_rate > 1, got nil")
		}
	})

	t.Run("ModeIO requires IOBytes > 0", func(t *testing.T) {
		cfg := Config{
			Mode:     ModeIO,
			Duration: time.Second,
			IOBytes:  0,
		}
		if err := validateConfig(cfg, DefaultLimits); err == nil {
			t.Fatalf("expected error for io_bytes <= 0 in io mode, got nil")
		}
	})
}
