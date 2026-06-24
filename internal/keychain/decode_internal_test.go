//nolint:testpackage // white-box test for the unexported decode helper
package keychain

import (
	"encoding/hex"
	"errors"
	"testing"
)

func TestDecodeRawJSON(t *testing.T) {
	got, err := decode([]byte("{\"claudeAiOauth\":{}}\n"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != `{"claudeAiOauth":{}}` {
		t.Fatalf("decode raw = %q", got)
	}
}

func TestDecodeHex(t *testing.T) {
	original := `{"claudeAiOauth":{"accessToken":"x"}}`
	encoded := hex.EncodeToString([]byte(original))
	got, err := decode([]byte(encoded + "\n"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != original {
		t.Fatalf("decode hex = %q, want %q", got, original)
	}
}

func TestDecodeEmpty(t *testing.T) {
	if _, err := decode([]byte("  \n")); !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}
