package keychain

import (
	"fmt"
	"os/exec"
	"strings"
)

const serviceName = "Claude Code-credentials"

// Write stores credentials in the macOS Keychain, replacing any existing entry.
func Write(data []byte) error {
	_ = exec.Command("security", "delete-generic-password", "-s", serviceName).Run()

	cmd := exec.Command("security", "add-generic-password",
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
