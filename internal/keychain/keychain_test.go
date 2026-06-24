//nolint:revive // string constants are clear in tests
package keychain_test

import (
	"testing"

	"github.com/mjmorales/claude-env/internal/keychain"
)

// TestServiceNameKnownVectors pins the hash scheme against values cracked from a
// real macOS Keychain (Claude Code 2.1.190). These are pure and platform-agnostic.
func TestServiceNameKnownVectors(t *testing.T) {
	cases := []struct {
		dir  string
		want string
	}{
		{"/Users/manuelmorales/.claude-envs/default", "Claude Code-credentials-4b8050b9"},
		{"/Users/manuelmorales/.claude-envs/max2", "Claude Code-credentials-f10fc24b"},
	}
	for _, c := range cases {
		if got := keychain.ServiceName(c.dir); got != c.want {
			t.Errorf("ServiceName(%q) = %q, want %q", c.dir, got, c.want)
		}
	}
}

// TestServiceNameCleansPath ensures a trailing slash does not change the result,
// matching how claude-env always passes a cleaned absolute path.
func TestServiceNameCleansPath(t *testing.T) {
	a := keychain.ServiceName("/Users/manuelmorales/.claude-envs/default")
	b := keychain.ServiceName("/Users/manuelmorales/.claude-envs/default/")
	if a != b {
		t.Fatalf("trailing slash changed service name: %q vs %q", a, b)
	}
}
