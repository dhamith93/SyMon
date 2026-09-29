package main

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/dhamith93/SyMon/internal/api"
	"github.com/dhamith93/SyMon/internal/update"
)

// fakeBuild is a script that answers -version like an agent would
func fakeBuild(version string) []byte {
	return []byte("#!/bin/sh\necho 'SyMon agent " + version + "'\n")
}

// signWithTestKey makes this test's agent trust a new key, and signs with it
func signWithTestKey(t *testing.T) func([]byte) []byte {
	t.Helper()
	private, public, err := update.NewKey()
	if err != nil {
		t.Fatal(err)
	}
	update.PublicKey = public
	t.Cleanup(func() { update.PublicKey = "" })
	key, _ := update.ParsePrivateKey(private)
	return func(build []byte) []byte { return []byte(update.Sign(key, build)) }
}

func TestInstallBuild(t *testing.T) {
	sign := signWithTestKey(t)
	exe := filepath.Join(t.TempDir(), "agent")
	if err := os.WriteFile(exe, fakeBuild("v1.0.0"), 0755); err != nil {
		t.Fatal(err)
	}
	unchanged := func() {
		t.Helper()
		if data, _ := os.ReadFile(exe); string(data) != string(fakeBuild("v1.0.0")) {
			t.Fatal("expected the agent to be left alone")
		}
		if _, err := os.Stat(exe + ".new"); !errors.Is(err, os.ErrNotExist) {
			t.Error("expected no leftover .new file")
		}
	}

	good := fakeBuild("v2.0.0")
	if err := installBuild(exe, good, sign([]byte("something else")), "v2.0.0"); err == nil || !strings.Contains(err.Error(), "not signed") {
		t.Errorf("expected a signature for another build to fail, got %v", err)
	}
	unchanged()
	if err := installBuild(exe, good, sign(good), "v3.0.0"); err == nil || !strings.Contains(err.Error(), `says "SyMon agent v2.0.0"`) {
		t.Errorf("expected a build of another version to fail, got %v", err)
	}
	unchanged()
	broken := []byte("not a program")
	if err := installBuild(exe, broken, sign(broken), "v2.0.0"); err == nil || !strings.Contains(err.Error(), "does not run here") {
		t.Errorf("expected a build that cannot run to fail, got %v", err)
	}
	unchanged()

	if err := installBuild(exe, good, sign(good), "v2.0.0"); err != nil {
		t.Fatal(err)
	}
	if data, _ := os.ReadFile(exe); string(data) != string(good) {
		t.Error("expected the new build in place")
	}
}

func TestInstallBuildNeedsAKey(t *testing.T) {
	update.PublicKey = ""
	exe := filepath.Join(t.TempDir(), "agent")
	if err := installBuild(exe, fakeBuild("v2.0.0"), []byte("x"), "v2.0.0"); !errors.Is(err, update.ErrNoKey) {
		t.Errorf("expected ErrNoKey, got %v", err)
	}
}

func TestFetchBuild(t *testing.T) {
	name := "/downloads/agent-linux-" + runtime.GOARCH
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case name:
			w.Write([]byte("build"))
		case name + ".sig":
			w.Write([]byte("signature"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	build, signature, err := fetchBuild(context.Background(), server.URL+"/")
	if err != nil || string(build) != "build" || string(signature) != "signature" {
		t.Errorf("expected the build and its signature, got %q %q %v", build, signature, err)
	}
	if _, _, err := fetchBuild(context.Background(), server.URL+"/elsewhere"); err == nil || !strings.Contains(err.Error(), "404") {
		t.Errorf("expected a missing build to fail, got %v", err)
	}
}

func TestUpdaterTriesEachRequestOnce(t *testing.T) {
	tries := 0
	updates := &updater{install: func(*api.AgentUpdate) error {
		tries++
		return errors.New("the build is not signed with this agent's update key")
	}}

	updates.handle(nil)
	updates.handle(&api.AgentUpdate{Version: "dev", RequestedAt: 1})
	if tries != 0 {
		t.Fatalf("expected no try without a request or for this version, got %d", tries)
	}
	request := &api.AgentUpdate{Version: "v9.0.0", DownloadUrl: "https://symon.example.com", RequestedAt: 1700000000}
	updates.handle(request)
	updates.handle(request)
	if tries != 1 || updates.err() == "" {
		t.Errorf("expected one try and its error kept, got %d %q", tries, updates.err())
	}
	// asking again on the dashboard is a new request
	updates.handle(&api.AgentUpdate{Version: "v9.0.0", RequestedAt: 1700000100})
	if tries != 2 {
		t.Errorf("expected a new request to be tried, got %d", tries)
	}
}
