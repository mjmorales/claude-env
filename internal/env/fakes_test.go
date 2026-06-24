package env_test

import (
	"errors"

	"github.com/mjmorales/claude-env/internal/config"
	"github.com/mjmorales/claude-env/internal/env"
	"github.com/mjmorales/claude-env/internal/fsutil"
)

// testNowMs is a fixed clock for deterministic expiry assertions.
const testNowMs int64 = 1_700_000_000_000

// fakeKeychain is an in-memory KeychainStore so auth flows can be exercised
// without shelling out to the macOS `security` binary.
type fakeKeychain struct {
	entries map[string][]byte
	deleted []string
	avail   bool
}

func newFakeKeychain(avail bool) *fakeKeychain {
	return &fakeKeychain{entries: map[string][]byte{}, avail: avail}
}

func (f *fakeKeychain) Available() bool { return f.avail }

func (f *fakeKeychain) Read(dir string) ([]byte, error) {
	if data, ok := f.entries[dir]; ok {
		return data, nil
	}
	return nil, errors.New("not found")
}

func (f *fakeKeychain) Delete(dir string) error {
	f.deleted = append(f.deleted, dir)
	delete(f.entries, dir)
	return nil
}

// fakeClaude is a stub ClaudeRunner. loginFunc simulates what `claude auth
// login` does to the keychain/filesystem for a config dir.
type fakeClaude struct {
	loginFunc  func(configDir string) error
	token      string
	unavailErr error
}

func (f *fakeClaude) Available() error { return f.unavailErr }

func (f *fakeClaude) Login(configDir string) error {
	if f.loginFunc != nil {
		return f.loginFunc(configDir)
	}
	return nil
}

func (f *fakeClaude) SetupToken(string) (string, error) { return f.token, nil }

// newTestManager builds a Manager wired with hermetic fakes and a fixed clock.
func newTestManager(paths config.Paths, cfg config.Config, fs *fsutil.SymlinkFs) *env.Manager {
	m := env.New(paths, cfg, fs)
	m.Keychain = newFakeKeychain(false)
	m.Claude = &fakeClaude{}
	m.Now = func() int64 { return testNowMs }
	return m
}
