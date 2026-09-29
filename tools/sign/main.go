// sign makes the key agent builds are signed with, and signs them. Agents
// install an update from the dashboard only when it is signed with this key.
//
//	go run ./tools/sign keygen -key ~/.config/symon/agent-signing.key
//	go run ./tools/sign sign -key ~/.config/symon/agent-signing.key agent-linux-amd64 ...
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
	flags.Parse(os.Args[2:])
	if *keyPath == "" {
		usage()
	}

	switch os.Args[1] {
	case "keygen":
		keygen(*keyPath)
	case "sign":
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
	fail("usage: sign keygen -key FILE | sign sign -key FILE BUILD...")
}
