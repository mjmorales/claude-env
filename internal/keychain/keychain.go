// Package keychain reads and removes the per-config-dir OAuth credential entry
// that Claude Code stores in the macOS Keychain.
//
// On macOS, Claude Code keys each CLAUDE_CONFIG_DIR to its own generic-password
// item named "Claude Code-credentials-<sha256(configDirAbsPath)[:8]>". claude-env
// uses Read to capture the token "claude auth login" writes there and Delete to
// purge it, after materializing the token into the env's .credentials.json file.
//
// It is read-only by design: restoring the default Claude Code flow writes a
// .credentials.json file instead of mutating the Keychain (see the
// keychain-read-only ADR). On non-macOS platforms every function is a no-op,
// since those platforms authenticate from files natively.
package keychain

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// servicePrefix is the constant portion of Claude Code's per-config-dir Keychain
// service name. The full name appends the first 8 hex chars of the SHA-256 of
// the config directory's absolute path.
const servicePrefix = "Claude Code-credentials-"

// serviceHashLen is how many leading hex characters of the config dir's SHA-256
// Claude Code appends to the service prefix.
const serviceHashLen = 8

// ErrUnsupported is returned by Read on platforms without a managed Keychain.
var ErrUnsupported = errors.New("keychain credential storage is only used on macOS")

// ErrNotFound is returned when no Keychain entry exists for a config directory.
var ErrNotFound = errors.New("no keychain entry for config dir")

// Available reports whether this platform stores Claude Code credentials in a
// Keychain that claude-env manages (macOS only).
func Available() bool {
	return runtime.GOOS == "darwin"
}

// ServiceName returns the Keychain service name Claude Code uses for a config
// directory. The directory is cleaned but not symlink-resolved, matching the
// path claude-env sets as CLAUDE_CONFIG_DIR.
func ServiceName(configDir string) string {
	sum := sha256.Sum256([]byte(filepath.Clean(configDir)))
	return servicePrefix + hex.EncodeToString(sum[:])[:serviceHashLen]
}

// Read returns the credential JSON bytes Claude Code stored for a config
// directory. Returns ErrUnsupported off macOS and ErrNotFound when no entry
// exists. The Keychain value is hex-decoded transparently when needed.
func Read(configDir string) ([]byte, error) {
	if !Available() {
		return nil, ErrUnsupported
	}
	svc := ServiceName(configDir)
	//nolint:gosec // svc is derived from a config path, not arbitrary input
	cmd := exec.CommandContext(context.Background(), "security",
		"find-generic-password", "-s", svc, "-w")
	out, err := cmd.Output()
	if err != nil {
		// `security` exits non-zero when the item is absent.
		return nil, ErrNotFound
	}
	return decode(out)
}

// Delete removes the Keychain entry for a config directory. A missing entry is
// not an error; off macOS it is a no-op.
func Delete(configDir string) error {
	if !Available() {
		return nil
	}
	svc := ServiceName(configDir)
	//nolint:gosec // svc is derived from a config path, not arbitrary input
	cmd := exec.CommandContext(context.Background(), "security",
		"delete-generic-password", "-s", svc)
	if err := cmd.Run(); err != nil {
		// Treat "item not found" (exit 44) as success; surface anything else.
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return nil
		}
		return fmt.Errorf("delete keychain entry: %w", err)
	}
	return nil
}

// decode normalizes a `security -w` value into credential JSON bytes. The tool
// prints a plain string when the password is printable and a hex dump otherwise,
// so a value that does not begin with '{' is treated as hex.
func decode(out []byte) ([]byte, error) {
	s := strings.TrimSpace(string(out))
	if s == "" {
		return nil, ErrNotFound
	}
	if s[0] == '{' {
		return []byte(s), nil
	}
	data, err := hex.DecodeString(s)
	if err != nil {
		return nil, fmt.Errorf("decode keychain value: %w", err)
	}
	return data, nil
}
