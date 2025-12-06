package load

import (
	"context"
	"errors"
	"fmt"
	"runtime"
	"sync"
	"time"
)

// Mode is a load generation mode.
type Mode string

const (
	ModeCPU    Mode = "cpu"
	ModeMem    Mode = "mem"
	ModeCPUMem Mode = "cpu-mem"
)

// Config holds parameters for load generation.
type Config struct {
	Mode        Mode
	Duration    time.Duration
	AllocMB     int // total memory to allocate in MB
	Parallelism int // number of CPU worker goroutines
}

// Limits defines safety upper bounds..
type Limits struct {
	MaxDuration    time.Duration
	MaxAllocMB     int
	MaxParallelism int
}

// DefaultLimits is a conservative default safety guard.
var DefaultLimits = Limits{
	MaxDuration:    60 * time.Second,
	MaxAllocMB:     512,
	MaxParallelism: runtime.NumCPU() * 4,
}

var (
	ErrInvalidMode        = errors.New("load: invalid mode")
	ErrDurationTooLarge   = errors.New("load:duration exceeds max ")
	ErrAllocTooLarge      = errors.New("load: alloc_mb exceeds max")
	ErrParallelismTooHigh = errors.New("load: parallelism exceeds max")
)

// Validation errors are returned immediately. Contextキャンセルやタイムアウトは
// 「想定された終了」とみなし、エラーは返さない
func Run(ctx context.Context, cfg Config) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := validateConfig(cfg, DefaultLimits); err != nil {
		return err
	}

	// Durationで自動終了するコンテキストに包む
	ctx, cancel := context.WithTimeout(ctx, cfg.Duration)
	defer cancel()

	var wg sync.WaitGroup

	switch cfg.Mode {
	case ModeCPU:
		startCPULoad(ctx, &wg, cfg.Parallelism)
	case ModeMem:
		startMemLoad(ctx, &wg, cfg.AllocMB)
	case ModeCPUMem:
		startCPULoad(ctx, &wg, cfg.Parallelism)
		startMemLoad(ctx, &wg, cfg.AllocMB)
	default:
		return fmt.Errorf("%w: %q", ErrInvalidMode, cfg.Mode)
	}

	// 全ワーカー終了を待つ
	wg.Wait()

	return nil
}

func validateConfig(cfg Config, limits Limits) error {
	if cfg.Duration <= 0 {
		return errors.New("load: duration must be > 0")
	}
	if limits.MaxDuration > 0 && cfg.Duration > limits.MaxDuration {
		return ErrDurationTooLarge
	}

	// モードごとに必要なパラメータをチェック
	switch cfg.Mode {
	case ModeCPU:
		if cfg.Parallelism <= 0 {
			return errors.New("load: parallelism must be > 0 for cpu mode")
		}
	case ModeMem:
		if cfg.AllocMB <= 0 {
			return errors.New("load: alloc_mb must be > 0 for mem mode")
		}
	case ModeCPUMem:
		if cfg.Parallelism <= 0 {
			return errors.New("load: parallelism must be > 0 for cpu-mem mode")
		}
		if cfg.AllocMB <= 0 {
			return errors.New("load: alloc_mb must be > 0 for cpu-mem mode")
		}
	default:
		return fmt.Errorf("%w: %q", ErrInvalidMode, cfg.Mode)
	}

	// 上限ガード
	if limits.MaxAllocMB > 0 && cfg.AllocMB > limits.MaxAllocMB {
		return ErrAllocTooLarge
	}
	if limits.MaxParallelism > 0 && cfg.Parallelism > limits.MaxParallelism {
		return ErrParallelismTooHigh
	}
	return nil
}

func startCPULoad(ctx context.Context, wg *sync.WaitGroup, n int) {
	if n <= 0 {
		return
	}

	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()

			// 適度にレジスタ/キャッシュを使う軽い計算
			var x float64
			for {
				select {
				case <-ctx.Done():
					return
				default:
					x += 1.0
					if x > 1e9 {
						x = 0
					}
				}
			}
		}()
	}
}

func startMemLoad(ctx context.Context, wg *sync.WaitGroup, allocMB int) {
	if allocMB <= 0 {
		return
	}
	wg.Add(1)
	go func() {
		defer wg.Done()

		// allocMB MBをチャンクに分けて確保する
		totalBytes := allocMB * 1024 * 1024
		const chunk = 1 * 1024 * 1024
		numChunks := (totalBytes + chunk - 1) / chunk

		bufs := make([][]byte, 0, numChunks)
		remaining := totalBytes

		for i := 0; i < numChunks; i++ {
			size := chunk
			if remaining < chunk {
				size = remaining
			}

			b := make([]byte, size)

			for j := 0; j < len(b); j += 4096 {
				b[j] = byte(j)
			}

			bufs = append(bufs, b)
			remaining -= size
		}
		<-ctx.Done()

		_ = bufs
	}()
}
