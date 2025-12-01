package codex

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"sync"
	"time"
)

// Options controls how we call Codex CLI.
type Options struct {
	// WorkDir is the directory Codex should treat as the workspace.
	// For Mu, default to the repo root.
	WorkDir string

	// ExtraArgs lets the caller override things like model, profile, etc.
	// Usually leave empty and let Codex config decide.
	ExtraArgs []string

	// Timeout is the maximum time for a single Codex run.
	Timeout time.Duration
}

// DefaultOptions provides safe defaults.
func DefaultOptions() Options {
	return Options{
		WorkDir:   ".",
		ExtraArgs: nil,
		Timeout:   90 * time.Second,
	}
}

// Ask sends a prompt to Codex in non-interactive mode and returns the
// final natural-language output printed to stdout.
//
// Authentication and billing are handled entirely by Codex, using the
// existing ChatGPT-based login or API key configured in ~/.codex.
func Ask(ctx context.Context, prompt string, opt Options) (string, error) {
	prompt = strings.TrimSpace(prompt)
	if prompt == "" {
		return "", errors.New("empty prompt")
	}
	if opt.Timeout <= 0 {
		opt.Timeout = 90 * time.Second
	}
	if opt.WorkDir == "" {
		opt.WorkDir = "."
	}

	if _, err := exec.LookPath("codex"); err != nil {
		return "", fmt.Errorf("codex binary not found on PATH: %w", err)
	}

	if err := ensureAuth(ctx); err != nil {
		return "", err
	}

	// Use non-interactive exec mode in read-only sandbox with approvals disabled.
	args := []string{
		"exec",
		"--sandbox", "read-only",
		"--cd", opt.WorkDir,
		prompt,
	}
	if len(opt.ExtraArgs) > 0 {
		args = append(args, opt.ExtraArgs...)
	}

	cmdCtx, cancel := context.WithTimeout(ctx, opt.Timeout)
	defer cancel()

	cmd := exec.CommandContext(cmdCtx, "codex", args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("codex exec failed: %w; stderr: %s", err, strings.TrimSpace(stderr.String()))
	}

	out := strings.TrimSpace(stdout.String())
	if out == "" {
		return "", errors.New("codex returned empty output")
	}

	return out, nil
}

var (
	authOnce sync.Once
	authErr  error
)

func ensureAuth(ctx context.Context) error {
	authOnce.Do(func() {
		// Check login status; this works non-interactively.
		if err := runCodexCommand(ctx, "login", "status"); err == nil {
			authErr = nil
			return
		}

		// Attempt interactive login (Codex will open browser/device auth as needed).
		if err := runCodexCommand(ctx, "login"); err != nil {
			authErr = fmt.Errorf("codex login failed: %w", err)
			return
		}

		authErr = nil
	})

	return authErr
}

func runCodexCommand(parent context.Context, args ...string) error {
	cmdCtx := parent
	if cmdCtx == nil {
		cmdCtx = context.Background()
	}
	// Cap command duration
	ctx, cancel := context.WithTimeout(cmdCtx, 60*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "codex", args...)
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%w: %s", err, strings.TrimSpace(buf.String()))
	}

	return nil
}
