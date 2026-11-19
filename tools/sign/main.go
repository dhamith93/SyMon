// sign makes the key agent builds are signed with, and signs them. Agents
// install an update from the dashboard only when it is signed with this key.
//
//	go run ./tools/sign keygen -key ~/.config/symon/agent-signing.key
//	go run ./tools/sign sign -key ~/.config/symon/agent-signing.key agent-linux-amd64 ...
//	go run ./tools/sign verify -pub deploy/agent-signing.pub agent-linux-amd64 ...
//
// verify checks each build against the .sig file next to it, with only the
// public key, so release builds can be checked where the key is not.
//
// keygen writes the public key next to the key, as <key>.pub, which the
// Makefile builds into agents. It leaves an existing key alone.
package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/dhamith93/SyMon/internal/update"
)

func main() {
	if len(os.Args) < 2 {
		usage()
	}
	flags := flag.NewFlagSet(os.Args[1], flag.ExitOnError)
	keyPath := flags.String("key", "", "the signing key file")
	publicPath := flags.String("pub", "", "with verify: the public key file")
	flags.Parse(os.Args[2:])

	switch {
	case os.Args[1] == "verify" && *publicPath != "":
		verify(*publicPath, flags.Args())
	case *keyPath == "":
		usage()
	case os.Args[1] == "keygen":
		keygen(*keyPath)
	case os.Args[1] == "sign":
		sign(*keyPath, flags.Args())
	default:
		usage()
	}
}

func keygen(path string) {
	if _, err := os.Stat(path); err == nil {
		return
	}
	private, public, err := update.NewKey()
	check(err)
	check(os.MkdirAll(filepath.Dir(path), 0700))
	check(os.WriteFile(path, []byte(private+"\n"), 0600))
	check(os.WriteFile(path+".pub", []byte(public+"\n"), 0644))
	fmt.Fprintf(os.Stderr, "Created the agent signing key %s. Back it up: agents only accept updates signed with it.\n", path)
}

func sign(path string, builds []string) {
	text, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		fail("there is no signing key at " + path + ", create one with: make signing-key")
	}
	check(err)
	key, err := update.ParsePrivateKey(string(text))
	check(err)
	for _, build := range builds {
		data, err := os.ReadFile(build)
		check(err)
		check(os.WriteFile(build+".sig", []byte(update.Sign(key, data)), 0644))
	}
}

func verify(path string, builds []string) {
	public, err := os.ReadFile(path)
	check(err)
	if len(builds) == 0 {
		usage()
	}
	for _, build := range builds {
		data, err := os.ReadFile(build)
		check(err)
		signature, err := os.ReadFile(build + ".sig")
		check(err)
		if err := update.VerifyWith(string(public), data, signature); err != nil {
			fail(build + ": " + err.Error())
		}
		fmt.Println(build + ": signed")
	}
}

func check(err error) {
	if err != nil {
		fail(err.Error())
	}
}

func fail(message string) {
	fmt.Fprintln(os.Stderr, message)
	os.Exit(1)
}

func usage() {
	fail("usage: sign keygen -key FILE | sign sign -key FILE BUILD... | sign verify -pub FILE BUILD...")
}
