package keychain

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
)

const serviceName = "Claude Code-credentials"

// Write stores credentials in the macOS Keychain, replacing any existing entry.
func Write(data []byte) error {
	// Attempt to delete any existing entry; ignore if it doesn't exist.
	//nolint:errcheck // deletion of non-existent entry fails but is not fatal
	_ = exec.CommandContext(context.Background(), "security", "delete-generic-password", "-s", serviceName).Run()

	//nolint:gosec // serviceName and data are not user-controlled
	cmd := exec.CommandContext(context.Background(), "security", "add-generic-password",
		"-s", serviceName,
		"-a", "",
		"-w", string(data),
		"-U",
	)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("write keychain: %s: %w", strings.TrimSpace(string(out)), err)
	}
	return nil
}
