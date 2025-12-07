package load

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"os"
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
	ModeIO     Mode = "io"
)

type Random interface {
	Float64() float64
}

// Config holds parameters for load generation.
type Config struct {
	Mode        Mode
	Duration    time.Duration
	AllocMB     int           // total memory to allocate in MB
	Parallelism int           // number of CPU worker goroutines
	IOBytes     int           // I/O負荷(ModeIOの時有効、1ループあたりに読み書きするバイト数)
	Latency     time.Duration // 固定遅延(全モード共通)、Run開始時にLatency分だけスリープする
	ErrorRate   float64       // 確率的エラー(全モード共通)、0.0~0.1の範囲でエラー発生確率を指定
	Rand        Random        // エラー注入用の乱数源(テストで差し替え可能)
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
	ErrInjected           = errors.New("load: injected error")
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

	// リクエスト単位の固定遅延を先頭で挿入
	maybeSleep(ctx, cfg.Latency)

	// 確率的エラー注入。trueの場合は負荷をかけずに即座に終了
	if shouldError(cfg) {
		return ErrInjected
	}

	var wg sync.WaitGroup

	switch cfg.Mode {
	case ModeCPU:
		startCPULoad(ctx, &wg, cfg.Parallelism)
	case ModeMem:
		startMemLoad(ctx, &wg, cfg.AllocMB)
	case ModeCPUMem:
		startCPULoad(ctx, &wg, cfg.Parallelism)
		startMemLoad(ctx, &wg, cfg.AllocMB)
	case ModeIO:
		startIOLoad(ctx, &wg, cfg.IOBytes)
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

	// 共通パラメータの検証
	if cfg.Latency < 0 {
		return errors.New("load: latency must be >= 0")
	}
	if cfg.ErrorRate < 0 || cfg.ErrorRate > 1 {
		return errors.New("load: error_rate must be between 0 and 1")
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
	case ModeIO:
		if cfg.IOBytes <= 0 {
			return errors.New("load: io_bytes must be > 0 for io mode")
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

func (c Config) rand() Random {
	if c.Rand != nil {
		return c.Rand
	}
	return rand.New(rand.NewSource(time.Now().UnixNano()))
}

func shouldError(cfg Config) bool {
	if cfg.ErrorRate <= 0 {
		return false
	}
	if cfg.ErrorRate >= 1 {
		return true
	}
	r := cfg.rand()
	v := r.Float64() // 0.0 <= v <1.0
	return v < cfg.ErrorRate
}

func maybeSleep(ctx context.Context, d time.Duration) {
	if d <= 0 {
		return
	}

	timer := time.NewTimer(d)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		// キャンセル/タイムアウトは「想定された終了」とみなし、エラーにしない
		return
	case <-timer.C:
		return
	}
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

// I/O負荷:一時ファイルに対してioBytesバイトの書き込みをDuration中ひたすら繰り返す。
func startIOLoad(ctx context.Context, wg *sync.WaitGroup, ioBytes int) {
	if ioBytes <= 0 {
		return
	}

	wg.Add(1)
	go func() {
		defer wg.Done()

		f, err := os.CreateTemp("", "cno-io-*")
		if err != nil {
			return
		}
		name := f.Name()
		defer func() {
			_ = f.Close()
			_ = os.Remove(name)
		}()

		const chunkSize = 32 * 1024
		buf := make([]byte, chunkSize)

		for {
			select {
			case <-ctx.Done():
				return
			default:
			}

			// ファイル先頭からioBytes分だけ書き込む
			if _, err := f.Seek(0, io.SeekStart); err != nil {
				return
			}

			remaining := ioBytes
			for remaining > 0 {
				select {
				case <-ctx.Done():
					return
				default:
				}

				n := remaining
				if n > len(buf) {
					n = len(buf)
				}
				if _, err := f.Write(buf[:n]); err != nil {
					return
				}
				remaining -= n
			}

			if err := f.Sync(); err != nil {
				return
			}
		}
	}()
}
