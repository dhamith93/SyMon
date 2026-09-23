package auth_test

import (
	"os"
	"testing"

	"github.com/dhamith93/SyMon/internal/auth"
)

func TestGetKeyGeneratesWhenUnset(t *testing.T) {
	t.Setenv("SYMON_KEY", "")
	os.Unsetenv("SYMON_KEY")

	key := auth.GetKey(true)
	if len(key) == 0 {
		t.Fatal("expected a generated key")
	}
	if os.Getenv("SYMON_KEY") != key {
		t.Error("generated key was not set in SYMON_KEY")
	}
}

func TestGetKeyEmptyWhenUnsetAndNoGenerate(t *testing.T) {
	t.Setenv("SYMON_KEY", "")
	os.Unsetenv("SYMON_KEY")

	if key := auth.GetKey(false); key != "" {
		t.Errorf("expected empty key, got: %s", key)
	}
}

func TestTokenRoundTrip(t *testing.T) {
	t.Setenv("SYMON_KEY", "test-key")

	token, err := auth.GenerateJWT()
	if err != nil {
		t.Fatal(err)
	}
	if !auth.ValidToken(token) {
		t.Error("expected token to be valid")
	}

	t.Setenv("SYMON_KEY", "other-key")
	if auth.ValidToken(token) {
		t.Error("expected token signed with another key to be invalid")
	}
}
