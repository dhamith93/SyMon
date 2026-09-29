package update

import (
	"strings"
	"testing"
)

func TestSignAndVerify(t *testing.T) {
	private, public, err := NewKey()
	if err != nil {
		t.Fatal(err)
	}
	key, err := ParsePrivateKey(private)
	if err != nil || PublicKeyOf(key) != public {
		t.Fatalf("expected the key to read back, got %v", err)
	}
	build := []byte("an agent build")
	signature := Sign(key, build)

	PublicKey = ""
	if err := Verify(build, []byte(signature)); err != ErrNoKey {
		t.Errorf("expected ErrNoKey without a key, got %v", err)
	}

	PublicKey = public
	t.Cleanup(func() { PublicKey = "" })
	if err := Verify(build, []byte(signature)); err != nil {
		t.Errorf("expected the signature to check out, got %v", err)
	}
	if err := Verify([]byte("another build"), []byte(signature)); err == nil || !strings.Contains(err.Error(), "not signed") {
		t.Errorf("expected a changed build to fail, got %v", err)
	}
	_, otherPublic, _ := NewKey()
	PublicKey = otherPublic
	if err := Verify(build, []byte(signature)); err == nil {
		t.Error("expected a build signed with another key to fail")
	}
	PublicKey = public
	if err := Verify(build, []byte("not a signature")); err == nil {
		t.Error("expected a garbled signature to fail")
	}
	if _, err := ParsePrivateKey("short"); err == nil {
		t.Error("expected a garbled key file to fail")
	}
}
