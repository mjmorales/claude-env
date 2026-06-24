// Package credentials models the Claude Code OAuth credential blob and reads
// and writes it as a .credentials.json file inside a CLAUDE_CONFIG_DIR.
//
// Claude Code authenticates from a single JSON object of the form
//
//	{"claudeAiOauth": {"accessToken": ..., "refreshToken": ..., ...}}
//
// When that file is present in CLAUDE_CONFIG_DIR it takes precedence over the
// macOS Keychain, which makes per-environment token files — rather than the
// opaque per-config-dir Keychain entry — the unit of identity for claude-env.
//
//nolint:revive // magic numbers are clear for file permissions
package credentials

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// FileName is the credential file Claude Code reads from a config directory.
const FileName = ".credentials.json"

// fileMode is the permission credential files are stored with. The blob
// contains long-lived refresh and access tokens, so it is owner-only.
const fileMode os.FileMode = 0o600

// FS is the filesystem surface credentials needs. *fsutil.SymlinkFs satisfies
// it, which keeps dry-run handling consistent with the rest of claude-env.
//
//nolint:interfacebloat // mirrors the subset of fsutil.SymlinkFs atomic writes require
type FS interface {
	ReadFile(name string) ([]byte, error)
	WriteFile(name string, data []byte, perm os.FileMode) error
	Chmod(name string, mode os.FileMode) error
	Stat(name string) (os.FileInfo, error)
	Remove(name string) error
	Rename(oldname, newname string) error
	MkdirAll(path string, perm os.FileMode) error
}

// OAuth is the inner credential object Claude Code stores under "claudeAiOauth".
// ExpiresAt is a Unix timestamp in milliseconds.
type OAuth struct {
	AccessToken      string   `json:"accessToken"`
	RefreshToken     string   `json:"refreshToken,omitempty"`
	ExpiresAt        int64    `json:"expiresAt,omitempty"`
	Scopes           []string `json:"scopes,omitempty"`
	SubscriptionType string   `json:"subscriptionType,omitempty"`
	RateLimitTier    string   `json:"rateLimitTier,omitempty"`
}

// Blob is the top-level credential document.
type Blob struct {
	ClaudeAiOauth OAuth `json:"claudeAiOauth"`
}

// ErrNotAuthenticated is returned when no credential file exists for a config dir.
var ErrNotAuthenticated = errors.New("no credentials (.credentials.json not found)")

// Path returns the credential file path for a config directory.
func Path(dir string) string {
	return filepath.Join(dir, FileName)
}

// Exists reports whether a config directory has a credential file.
func Exists(fs FS, dir string) bool {
	_, err := fs.Stat(Path(dir))
	return err == nil
}

// Parse decodes a credential blob from raw JSON bytes.
func Parse(data []byte) (Blob, error) {
	var b Blob
	if err := json.Unmarshal(data, &b); err != nil {
		return Blob{}, fmt.Errorf("parse credentials: %w", err)
	}
	return b, nil
}

// Validate rejects a blob that Claude Code could not authenticate with.
func Validate(b Blob) error {
	if b.ClaudeAiOauth.AccessToken == "" {
		return errors.New("credential blob has no claudeAiOauth.accessToken")
	}
	return nil
}

// ValidateRaw parses and validates raw bytes, returning the typed view. It is
// the gate every imported or captured blob passes through before being written.
func ValidateRaw(data []byte) (Blob, error) {
	b, err := Parse(data)
	if err != nil {
		return Blob{}, err
	}
	if err := Validate(b); err != nil {
		return Blob{}, err
	}
	return b, nil
}

// ReadRaw returns the credential file's raw bytes, preserving any fields not
// modeled by Blob. Returns ErrNotAuthenticated if the file is absent.
func ReadRaw(fs FS, dir string) ([]byte, error) {
	if !Exists(fs, dir) {
		return nil, ErrNotAuthenticated
	}
	data, err := fs.ReadFile(Path(dir))
	if err != nil {
		return nil, fmt.Errorf("read credentials: %w", err)
	}
	return data, nil
}

// Read returns the typed credential blob for a config directory.
func Read(fs FS, dir string) (Blob, error) {
	data, err := ReadRaw(fs, dir)
	if err != nil {
		return Blob{}, err
	}
	return Parse(data)
}

// WriteRaw validates and atomically writes raw credential bytes (temp + rename)
// with 0600 permissions. Writing the original bytes preserves forward-compatible
// fields Blob does not model, so capture and import are lossless.
func WriteRaw(fs FS, dir string, data []byte) error {
	if _, err := ValidateRaw(data); err != nil {
		return err
	}
	if err := fs.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}

	dst := Path(dir)
	tmp := dst + ".tmp"
	if err := fs.WriteFile(tmp, data, fileMode); err != nil {
		return fmt.Errorf("write credentials temp: %w", err)
	}
	// WriteFile honors perm on create, but an inherited umask can loosen it;
	// chmod makes the owner-only mode explicit before the file is published.
	if err := fs.Chmod(tmp, fileMode); err != nil {
		//nolint:errcheck // best-effort cleanup; the chmod error is what matters
		_ = fs.Remove(tmp)
		return fmt.Errorf("chmod credentials temp: %w", err)
	}
	if err := fs.Rename(tmp, dst); err != nil {
		//nolint:errcheck // best-effort cleanup of the orphaned temp file
		_ = fs.Remove(tmp)
		return fmt.Errorf("rename credentials into place: %w", err)
	}
	return nil
}

// Write marshals and atomically writes a credential blob. Used for synthesized
// blobs (e.g. wrapping a setup-token); prefer WriteRaw when round-tripping an
// existing blob to avoid dropping unmodeled fields.
func Write(fs FS, dir string, b Blob) error {
	if err := Validate(b); err != nil {
		return err
	}
	data, err := json.MarshalIndent(b, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal credentials: %w", err)
	}
	return WriteRaw(fs, dir, data)
}

// Delete removes the credential file for a config directory. A missing file is
// not an error.
func Delete(fs FS, dir string) error {
	if !Exists(fs, dir) {
		return nil
	}
	if err := fs.Remove(Path(dir)); err != nil {
		return fmt.Errorf("delete credentials: %w", err)
	}
	return nil
}

// Expired reports whether the access token is expired at nowMs (Unix millis).
// A blob without an expiry is treated as non-expiring.
func (o OAuth) Expired(nowMs int64) bool {
	return o.ExpiresAt > 0 && nowMs >= o.ExpiresAt
}

// ExpiresIn returns the duration until the access token expires relative to
// nowMs. It is negative once expired and zero when no expiry is set.
func (o OAuth) ExpiresIn(nowMs int64) time.Duration {
	if o.ExpiresAt == 0 {
		return 0
	}
	return time.Duration(o.ExpiresAt-nowMs) * time.Millisecond
}

// Redacted returns a copy of the blob with token material masked for display.
func (b Blob) Redacted() Blob {
	r := b
	r.ClaudeAiOauth.AccessToken = mask(b.ClaudeAiOauth.AccessToken)
	r.ClaudeAiOauth.RefreshToken = mask(b.ClaudeAiOauth.RefreshToken)
	return r
}

// mask replaces all but the last four characters of a secret with bullets,
// keeping enough to distinguish tokens without revealing them.
func mask(s string) string {
	if s == "" {
		return ""
	}
	const keep = 4
	if len(s) <= keep {
		return "****"
	}
	return "****" + s[len(s)-keep:]
}
