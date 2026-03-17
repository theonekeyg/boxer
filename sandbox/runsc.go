package sandbox

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"time"

	"github.com/rs/zerolog/log"

	"boxer/config"
	"boxer/internal/ioutil"
)

// Executor runs OCI bundles inside gVisor via runsc.
type Executor struct {
	cfg *config.BoxerConfig
}

// NewExecutor constructs an Executor from the given config.
func NewExecutor(cfg *config.BoxerConfig) *Executor {
	return &Executor{cfg: cfg}
}

// Result holds the output of a completed sandbox execution.
type Result struct {
	ExitCode int
	Stdout   []byte
	Stderr   []byte
	WallMs   int64
}

// ErrTimeout is returned when the sandbox exceeds its wall-clock limit.
var ErrTimeout = errors.New("execution timed out")

// ErrOutputLimit is returned when stdout or stderr exceeds the configured limit.
var ErrOutputLimit = errors.New("output limit exceeded")

// Run executes the OCI bundle in the given BundleDir inside a gVisor sandbox.
// The caller should set a context deadline matching the wall-clock limit.
func (e *Executor) Run(ctx context.Context, bundle *BundleDir, limits config.ResourceLimits) (*Result, error) {
	if err := os.MkdirAll(bundle.RunscRoot(), 0o755); err != nil {
		return nil, fmt.Errorf("create runsc state dir: %w", err)
	}

	//nolint:gosec // the path comes from trusted config
	cmd := exec.CommandContext(ctx,
		e.cfg.RunscPath,
		"--root", bundle.RunscRoot(),
		"--platform", e.cfg.Platform,
		"--network=none",
		"--log-format=text",
		"run",
		"--bundle", bundle.BundlePath(),
		bundle.ExecID,
	)

	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("stdout pipe: %w", err)
	}
	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		return nil, fmt.Errorf("stderr pipe: %w", err)
	}

	start := time.Now()
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start runsc: %w", err)
	}

	limit := e.cfg.OutputLimitBytes

	type readResult struct {
		data []byte
		err  error
	}

	stdoutCh := make(chan readResult, 1)
	stderrCh := make(chan readResult, 1)

	go func() {
		data, err := ioutil.ReadLimited(stdoutPipe, limit)
		stdoutCh <- readResult{data, err}
	}()
	go func() {
		data, err := ioutil.ReadLimited(stderrPipe, limit)
		stderrCh <- readResult{data, err}
	}()

	waitErr := cmd.Wait()
	wallMs := time.Since(start).Milliseconds()

	stdoutRes := <-stdoutCh
	stderrRes := <-stderrCh

	// Drain any residual data in pipes after Wait (pipe goroutines have finished).
	io.Copy(io.Discard, stdoutPipe) //nolint:errcheck
	io.Copy(io.Discard, stderrPipe) //nolint:errcheck

	// Check context for timeout.
	if ctx.Err() == context.DeadlineExceeded {
		killSandbox(e.cfg, bundle)
		return nil, fmt.Errorf("%w after %dms", ErrTimeout, wallMs)
	}
	if waitErr != nil && ctx.Err() != nil {
		killSandbox(e.cfg, bundle)
		return nil, fmt.Errorf("%w after %dms", ErrTimeout, wallMs)
	}

	exitCode := 0
	if waitErr != nil {
		var exitErr *exec.ExitError
		if errors.As(waitErr, &exitErr) {
			exitCode = exitErr.ExitCode()
			if exitCode == -1 {
				exitCode = 1
			}
		} else {
			return nil, fmt.Errorf("runsc wait: %w", waitErr)
		}
	}

	if stdoutRes.err != nil {
		return nil, fmt.Errorf("read stdout: %w", stdoutRes.err)
	}
	if stderrRes.err != nil {
		return nil, fmt.Errorf("read stderr: %w", stderrRes.err)
	}

	// Detect truncation: if we read exactly the limit, the stream was cut short.
	if len(stdoutRes.data) == limit || len(stderrRes.data) == limit {
		return nil, fmt.Errorf("%w: limit=%d bytes", ErrOutputLimit, limit)
	}

	log.Debug().Str("exec_id", bundle.ExecID).Int("exit_code", exitCode).Int64("wall_ms", wallMs).Msg("runsc complete")

	return &Result{
		ExitCode: exitCode,
		Stdout:   stdoutRes.data,
		Stderr:   stderrRes.data,
		WallMs:   wallMs,
	}, nil
}

// killSandbox sends SIGKILL to a timed-out container (best-effort).
func killSandbox(cfg *config.BoxerConfig, bundle *BundleDir) {
	//nolint:gosec
	cmd := exec.Command(cfg.RunscPath,
		"--root", bundle.RunscRoot(),
		"kill", bundle.ExecID, "SIGKILL",
	)
	if err := cmd.Run(); err != nil {
		log.Warn().Err(err).Str("exec_id", bundle.ExecID).Msg("runsc kill failed")
	}
}
