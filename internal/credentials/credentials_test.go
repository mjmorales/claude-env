//nolint:revive // magic numbers and string constants OK in tests
package credentials_test

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/mjmorales/claude-env/internal/credentials"
	"github.com/mjmorales/claude-env/internal/fsutil"
)

func sampleBlob() credentials.Blob {
	return credentials.Blob{ClaudeAiOauth: credentials.OAuth{
		AccessToken:      "sk-ant-oat01-access",
		RefreshToken:     "sk-ant-ort01-refresh",
		ExpiresAt:        1_700_000_000_000,
		Scopes:           []string{"user:inference", "user:profile"},
		SubscriptionType: "max",
		RateLimitTier:    "default_claude_ai",
	}}
}

func TestWriteReadRoundTrip(t *testing.T) {
	fs := fsutil.NewOs(false)
	dir := t.TempDir()

	if err := credentials.Write(fs, dir, sampleBlob()); err != nil {
		t.Fatalf("Write: %v", err)
	}

	got, err := credentials.Read(fs, dir)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	want := sampleBlob()
	if got.ClaudeAiOauth.AccessToken != want.ClaudeAiOauth.AccessToken ||
		got.ClaudeAiOauth.RefreshToken != want.ClaudeAiOauth.RefreshToken ||
		got.ClaudeAiOauth.ExpiresAt != want.ClaudeAiOauth.ExpiresAt ||
		got.ClaudeAiOauth.SubscriptionType != want.ClaudeAiOauth.SubscriptionType ||
		got.ClaudeAiOauth.RateLimitTier != want.ClaudeAiOauth.RateLimitTier {
		t.Fatalf("round trip mismatch: got %+v", got.ClaudeAiOauth)
	}
}

func TestWriteEnforces0600(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unix permission bits not meaningful on windows")
	}
	fs := fsutil.NewOs(false)
	dir := t.TempDir()

	if err := credentials.Write(fs, dir, sampleBlob()); err != nil {
		t.Fatalf("Write: %v", err)
	}

	info, err := os.Stat(credentials.Path(dir))
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Fatalf("perm = %o, want 600", perm)
	}
	// The temp file must not be left behind.
	if _, err := os.Stat(credentials.Path(dir) + ".tmp"); !os.IsNotExist(err) {
		t.Fatalf("temp file left behind: %v", err)
	}
}

func TestWriteRawPreservesUnknownFields(t *testing.T) {
	fs := fsutil.NewOs(false)
	dir := t.TempDir()

	raw := []byte(`{"claudeAiOauth":{"accessToken":"tok","futureField":42},"otherTop":"keep"}`)
	if err := credentials.WriteRaw(fs, dir, raw); err != nil {
		t.Fatalf("WriteRaw: %v", err)
	}

	data, err := credentials.ReadRaw(fs, dir)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatal(err)
	}
	if _, ok := m["otherTop"]; !ok {
		t.Fatal("WriteRaw dropped top-level field 'otherTop'")
	}
	var oauth map[string]json.RawMessage
	if err := json.Unmarshal(m["claudeAiOauth"], &oauth); err != nil {
		t.Fatal(err)
	}
	if _, ok := oauth["futureField"]; !ok {
		t.Fatal("WriteRaw dropped nested field 'futureField'")
	}
}

func TestReadMissingReturnsSentinel(t *testing.T) {
	fs := fsutil.NewOs(false)
	if _, err := credentials.Read(fs, t.TempDir()); !errors.Is(err, credentials.ErrNotAuthenticated) {
		t.Fatalf("err = %v, want ErrNotAuthenticated", err)
	}
}

func TestValidateRejectsEmptyAccessToken(t *testing.T) {
	if err := credentials.Validate(credentials.Blob{}); err == nil {
		t.Fatal("expected error for empty access token")
	}
	if err := credentials.Validate(sampleBlob()); err != nil {
		t.Fatalf("unexpected error for valid blob: %v", err)
	}
}

func TestWriteRawRejectsInvalid(t *testing.T) {
	fs := fsutil.NewOs(false)
	dir := t.TempDir()

	if err := credentials.WriteRaw(fs, dir, []byte(`{"claudeAiOauth":{}}`)); err == nil {
		t.Fatal("expected error writing blob with no access token")
	}
	// Nothing should have been written.
	if credentials.Exists(fs, dir) {
		t.Fatal("invalid blob should not have produced a file")
	}
}

func TestExpiry(t *testing.T) {
	o := credentials.OAuth{ExpiresAt: 1000}
	if o.Expired(999) {
		t.Fatal("should not be expired before expiry")
	}
	if !o.Expired(1000) {
		t.Fatal("should be expired at expiry")
	}
	if !o.Expired(1001) {
		t.Fatal("should be expired after expiry")
	}
	if got := o.ExpiresIn(900); got != 100*1_000_000 /* 100ms in ns */ {
		t.Fatalf("ExpiresIn = %v, want 100ms", got)
	}

	// No expiry set => never expires, zero remaining.
	none := credentials.OAuth{}
	if none.Expired(1 << 62) {
		t.Fatal("blob without expiry should never be expired")
	}
	if none.ExpiresIn(123) != 0 {
		t.Fatal("blob without expiry should report zero remaining")
	}
}

func TestRedactedMasksTokens(t *testing.T) {
	r := sampleBlob().Redacted()
	if r.ClaudeAiOauth.AccessToken != "****cess" {
		t.Fatalf("access token = %q, want ****cess", r.ClaudeAiOauth.AccessToken)
	}
	if r.ClaudeAiOauth.RefreshToken != "****resh" {
		t.Fatalf("refresh token = %q, want ****resh", r.ClaudeAiOauth.RefreshToken)
	}
	// Non-secret fields are untouched.
	if r.ClaudeAiOauth.SubscriptionType != "max" {
		t.Fatalf("subscriptionType = %q, want max", r.ClaudeAiOauth.SubscriptionType)
	}
	// Short secrets are fully masked.
	short := credentials.Blob{ClaudeAiOauth: credentials.OAuth{AccessToken: "ab"}}.Redacted()
	if short.ClaudeAiOauth.AccessToken != "****" {
		t.Fatalf("short token = %q, want ****", short.ClaudeAiOauth.AccessToken)
	}
}

func TestPath(t *testing.T) {
	dir := t.TempDir()
	want := filepath.Join(dir, credentials.FileName)
	if got := credentials.Path(dir); got != want {
		t.Fatalf("Path = %q, want %q", got, want)
	}
}
