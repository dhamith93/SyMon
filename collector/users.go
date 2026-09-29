package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/dhamith93/SyMon/internal/config"
	"github.com/dhamith93/SyMon/internal/store"
)

// Dashboard users are managed from the command line. A new password is
// made up and printed once, unless -password-stdin gives one.

func addUser(ctx context.Context, st *store.Store, config *config.Collector, name string, fromStdin bool) {
	migrateOrExit(ctx, st)
	password := newPassword(fromStdin)
	err := st.AddUser(ctx, name, password)
	if errors.Is(err, store.ErrUserExists) {
		exit("User " + name + " already exists. Use -reset-password to give it a new password.")
	}
	if err != nil {
		exit("cannot add the user: " + err.Error())
	}
	fmt.Printf("Created user %s.\n", name)
	if !fromStdin {
		printPassword(password)
	}
	fmt.Println("Log in at " + dashboardURL(config))
}

func resetPassword(ctx context.Context, st *store.Store, name string, fromStdin bool) {
	migrateOrExit(ctx, st)
	password := newPassword(fromStdin)
	err := st.SetPassword(ctx, name, password)
	if errors.Is(err, store.ErrNotFound) {
		exit("There is no user " + name + ".")
	}
	if err != nil {
		exit("cannot set the password: " + err.Error())
	}
	fmt.Printf("Set a new password for %s and logged them out everywhere.\n", name)
	if !fromStdin {
		printPassword(password)
	}
}

func removeUser(ctx context.Context, st *store.Store, name string) {
	migrateOrExit(ctx, st)
	err := st.RemoveUser(ctx, name)
	if errors.Is(err, store.ErrNotFound) {
		exit("There is no user " + name + ".")
	}
	if err != nil {
		exit("cannot remove the user: " + err.Error())
	}
	fmt.Printf("Removed user %s and logged them out everywhere.\n", name)
}

func listUsers(ctx context.Context, st *store.Store) {
	migrateOrExit(ctx, st)
	users, err := st.Users(ctx)
	if err != nil {
		exit("cannot list users: " + err.Error())
	}
	if len(users) == 0 {
		fmt.Println("No users yet, so the dashboard is locked. Add one with -add-user <name>.")
		return
	}
	table := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(table, "USER\tCREATED\tLAST LOGIN")
	for _, user := range users {
		lastLogin := "never"
		if !user.LastLogin.IsZero() {
			lastLogin = user.LastLogin.Local().Format("Jan 2 2006 15:04")
		}
		fmt.Fprintf(table, "%s\t%s\t%s\n", user.Name, user.CreatedAt.Local().Format("Jan 2 2006"), lastLogin)
	}
	table.Flush()
}

// newPassword reads a password from the first line of stdin, or makes one up
func newPassword(fromStdin bool) string {
	if !fromStdin {
		password, err := store.NewPassword()
		if err != nil {
			exit("cannot make a password: " + err.Error())
		}
		return password
	}
	line, err := bufio.NewReader(os.Stdin).ReadString('\n')
	if err != nil && line == "" {
		exit("cannot read a password from stdin: " + err.Error())
	}
	return strings.TrimRight(line, "\r\n")
}

func printPassword(password string) {
	fmt.Printf("\nPassword, shown only this once:\n\n  %s\n\n", password)
}

// migrateOrExit creates the user tables if the collector has not run since
// an upgrade
func migrateOrExit(ctx context.Context, st *store.Store) {
	ctx, cancel := context.WithTimeout(ctx, time.Minute)
	defer cancel()
	if err := st.Migrate(ctx); err != nil {
		exit("cannot update database schema: " + err.Error())
	}
}

func exit(message string) {
	fmt.Fprintln(os.Stderr, message)
	os.Exit(1)
}
