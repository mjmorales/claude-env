package env

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/mjmorales/claude-env/internal/keychain"
)

// ClaudeRunner runs the claude CLI with CLAUDE_CONFIG_DIR pointed at a config
// directory. It is an interface so the auth flows can be tested without the real
// binary or an interactive browser login.
type ClaudeRunner interface {
	// Available reports whether the claude binary can be found.
	Available() error
	// Login runs an interactive `claude auth login`, inheriting stdio.
	Login(configDir string) error
	// SetupToken runs `claude setup-token` and returns the printed token.
	SetupToken(configDir string) (string, error)
}

// KeychainStore captures and removes the per-config-dir credential entry Claude
// Code writes to the macOS Keychain. It is an interface so tests can stub it.
type KeychainStore interface {
	Available() bool
	Read(configDir string) ([]byte, error)
	Delete(configDir string) error
}

// ExecClaudeRunner is the production ClaudeRunner backed by the claude binary.
type ExecClaudeRunner struct{}

// Available reports whether the claude binary is on PATH.
func (ExecClaudeRunner) Available() error {
	if _, err := exec.LookPath("claude"); err != nil {
		return fmt.Errorf("claude CLI not found in PATH: %w", err)
	}
	return nil
}

// Login runs `claude auth login` under configDir with inherited stdio so the
// user can complete the interactive OAuth flow.
func (ExecClaudeRunner) Login(configDir string) error {
	bin, err := exec.LookPath("claude")
	if err != nil {
		return fmt.Errorf("claude CLI not found in PATH: %w", err)
	}
	//nolint:gosec // bin is resolved by exec.LookPath
	c := exec.CommandContext(context.Background(), bin, "auth", "login")
	c.Env = append(os.Environ(), "CLAUDE_CONFIG_DIR="+configDir)
	c.Stdin = os.Stdin
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	if err := c.Run(); err != nil {
		return fmt.Errorf("run claude auth login: %w", err)
	}
	return nil
}

// SetupToken runs `claude setup-token` under configDir, passing stdin/stderr
// through for the interactive flow while capturing the token from stdout.
func (ExecClaudeRunner) SetupToken(configDir string) (string, error) {
	bin, err := exec.LookPath("claude")
	if err != nil {
		return "", fmt.Errorf("claude CLI not found in PATH: %w", err)
	}
	var out bytes.Buffer
	//nolint:gosec // bin is resolved by exec.LookPath
	c := exec.CommandContext(context.Background(), bin, "setup-token")
	c.Env = append(os.Environ(), "CLAUDE_CONFIG_DIR="+configDir)
	c.Stdin = os.Stdin
	c.Stderr = os.Stderr
	c.Stdout = &out
	if err := c.Run(); err != nil {
		return "", fmt.Errorf("run claude setup-token: %w", err)
	}
	token := extractToken(out.String())
	if token == "" {
		return "", fmt.Errorf("claude setup-token produced no token")
	}
	return token, nil
}

// extractToken pulls an sk-ant-* token out of setup-token's output, tolerating
// surrounding instructional text.
func extractToken(s string) string {
	for _, line := range strings.Fields(s) {
		if strings.HasPrefix(line, "sk-ant-") {
			return line
		}
	}
	return strings.TrimSpace(s)
}

// keychainAdapter is the production KeychainStore backed by the keychain package.
// Its methods are thin passthroughs; the keychain package already returns
// descriptive errors, so they are not re-wrapped.
type keychainAdapter struct{}

func (keychainAdapter) Available() bool { return keychain.Available() }

//nolint:wrapcheck // thin passthrough; keychain errors are already descriptive
func (keychainAdapter) Read(dir string) ([]byte, error) { return keychain.Read(dir) }

//nolint:wrapcheck // thin passthrough; keychain errors are already descriptive
func (keychainAdapter) Delete(dir string) error { return keychain.Delete(dir) }
