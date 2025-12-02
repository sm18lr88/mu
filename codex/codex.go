package codex

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
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

	// Model sets the Codex model (e.g., "gpt-5.1-codex", "gpt-5.1-codex-max").
	Model string

	// ReasoningLevel sets the Codex "thinking" effort (e.g., "default", "medium", "high").
	ReasoningLevel string

	// ExtraArgs lets the caller override things like model, profile, etc.
	// Usually leave empty and let Codex config decide.
	ExtraArgs []string

	// Timeout is the maximum time for a single Codex run.
	Timeout time.Duration
}

// DefaultOptions provides safe defaults.
func DefaultOptions() Options {
	return Options{
		WorkDir:        ".",
		Model:          "",
		ReasoningLevel: "",
		ExtraArgs:      nil,
		Timeout:        90 * time.Second,
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

	cmdName, wrapperArgs, err := resolveCodexCommand()
	if err != nil {
		return "", err
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

	cmdArgs := append(wrapperArgs, args...)

	if opt.Model != "" {
		cmdArgs = append(cmdArgs, "--model", opt.Model)
	}
	if rl := strings.TrimSpace(opt.ReasoningLevel); rl != "" {
		cmdArgs = append(cmdArgs, "--thinking", rl)
	}

	cmd := exec.CommandContext(cmdCtx, cmdName, cmdArgs...)
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

	cmdName, wrapperArgs, err := resolveCodexCommand()
	if err != nil {
		return err
	}
	cmdArgs := append(wrapperArgs, args...)
	cmd := exec.CommandContext(ctx, cmdName, cmdArgs...)
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%w: %s", err, strings.TrimSpace(buf.String()))
	}

	return nil
}

// commandName resolves the codex command name (or path) from env override.
func commandName() string {
	if v := strings.TrimSpace(os.Getenv("MU_CODEX_CMD")); v != "" {
		return v
	}
	return "codex"
}

// HasCodex reports whether a codex command is discoverable via PATH or MU_CODEX_CMD.
func HasCodex() bool {
	_, _, err := resolveCodexCommand()
	return err == nil
}

// resolveCodexCommand finds the command used to invoke Codex.
// It supports .exe/.cmd/.bat and PowerShell shims (.ps1) by wrapping with pwsh/powershell.
func resolveCodexCommand() (string, []string, error) {
	name := commandName()

	// If the env provides a direct path, honor it.
	if strings.ContainsAny(name, `\/`) {
		return buildCommand(name)
	}

	// Try common Windows extensions plus bare name (Unix)
	paths := []string{
		name + ".exe",
		name + ".cmd",
		name + ".bat",
		name + ".ps1",
		name, // last resort (Unix-style shim)
	}

	for _, candidate := range paths {
		if p, err := exec.LookPath(candidate); err == nil {
			return buildCommand(p)
		}
	}

	return "", nil, fmt.Errorf("codex binary not found on PATH (tried %q)", strings.Join(paths, ", "))
}

func buildCommand(path string) (string, []string, error) {
	lower := strings.ToLower(path)
	if strings.HasSuffix(lower, ".ps1") {
		shell := firstAvailable([]string{"pwsh", "powershell"})
		if shell == "" {
			return "", nil, fmt.Errorf("found Codex PowerShell shim at %s but no pwsh/powershell available", path)
		}
		return shell, []string{"-NoLogo", "-NoProfile", "-ExecutionPolicy", "Bypass", "-File", path}, nil
	}
	return path, nil, nil
}

func firstAvailable(names []string) string {
	for _, n := range names {
		if _, err := exec.LookPath(n); err == nil {
			return n
		}
	}
	return ""
}
