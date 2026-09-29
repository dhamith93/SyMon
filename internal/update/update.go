// Package update signs agent builds and checks them before an agent
// installs one. Builds are signed with an ed25519 key that stays where they
// are built, so a build an agent accepts cannot come from anyone else.
package update

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
)

// PublicKey is the base64 key agent builds are signed with, set at build
// time. An agent built without it does not install updates.
var PublicKey = ""

// ErrNoKey is an agent built without PublicKey
var ErrNoKey = errors.New("this agent was built without an update key, update it with the install command")

// Verify checks a build against its signature file, which holds the
// base64 signature
func Verify(build []byte, signature []byte) error {
	if PublicKey == "" {
		return ErrNoKey
	}
	key, err := base64.StdEncoding.DecodeString(PublicKey)
	if err != nil || len(key) != ed25519.PublicKeySize {
		return errors.New("this agent's update key is broken")
	}
	sig, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(signature)))
	if err != nil || len(sig) != ed25519.SignatureSize {
		return errors.New("the signature file is not a signature")
	}
	if !ed25519.Verify(ed25519.PublicKey(key), build, sig) {
		return errors.New("the build is not signed with this agent's update key")
	}
	return nil
}

// NewKey returns a private key as the text kept in the key file, and its
// public key
func NewKey() (private string, public string, err error) {
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return "", "", err
	}
	return base64.StdEncoding.EncodeToString(privateKey.Seed()), base64.StdEncoding.EncodeToString(publicKey), nil
}

// ParsePrivateKey reads the text of a key file
func ParsePrivateKey(text string) (ed25519.PrivateKey, error) {
	seed, err := base64.StdEncoding.DecodeString(strings.TrimSpace(text))
	if err != nil || len(seed) != ed25519.SeedSize {
		return nil, fmt.Errorf("not a SyMon signing key")
	}
	return ed25519.NewKeyFromSeed(seed), nil
}

// Sign returns the text of a build's signature file
func Sign(key ed25519.PrivateKey, build []byte) string {
	return base64.StdEncoding.EncodeToString(ed25519.Sign(key, build)) + "\n"
}

// PublicKeyOf returns the base64 public key of a private key
func PublicKeyOf(key ed25519.PrivateKey) string {
	return base64.StdEncoding.EncodeToString(key.Public().(ed25519.PublicKey))
}
