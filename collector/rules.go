package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/dhamith93/SyMon/internal/alerts"
	"github.com/dhamith93/SyMon/internal/config"
	"github.com/dhamith93/SyMon/internal/logger"
	"github.com/dhamith93/SyMon/internal/store"
)

// setupRules moves the alert rules into the database once: it imports
// alerts.json and adds the default heartbeat rule. A broken file stops the
// collector, so rules are never dropped quietly.
func setupRules(ctx context.Context, st *store.Store, config *config.Collector) {
	done, err := st.RulesSetUp(ctx)
	if err != nil {
		log.Fatal("cannot read the alert rules: ", err)
	}
	if done {
		if config.AlertsFilePath != "" {
			logger.Log("info", config.AlertsFilePath+" is no longer read, alert rules are edited on the dashboard. Remove SYMON_ALERTS_CONFIG_PATH, or load a file with -import-rules.")
		}
		return
	}

	var rules []alerts.AlertConfig
	if config.AlertsFilePath != "" {
		rules, err = alerts.LoadRules(config.AlertsFilePath)
		if err != nil {
			log.Fatal("cannot import the alert rules, fix the file or unset SYMON_ALERTS_CONFIG_PATH: ", err)
		}
	}
	setup, err := st.SetupRules(ctx, rules, config.AlertsFilePath)
	if err != nil {
		log.Fatal("cannot import the alert rules: ", err)
	}
	if setup.Done {
		return
	}
	changes := []string{}
	if config.AlertsFilePath != "" {
		changes = append(changes, fmt.Sprintf("imported %d from %s", setup.Imported, config.AlertsFilePath))
	}
	if len(setup.Added) > 0 {
		changes = append(changes, "added "+strings.Join(setup.Added, ", "))
	}
	message := "alert rules now live in the database and are edited on the dashboard"
	if len(changes) > 0 {
		message += ": " + strings.Join(changes, ", ")
	}
	logger.Log("info", message)
}

// importRules loads a file into the database while the collector runs,
// which applies it within 15 seconds
func importRules(ctx context.Context, st *store.Store, path string) {
	migrateOrExit(ctx, st)
	rules, err := alerts.LoadRules(path)
	if err != nil {
		exit(err.Error())
	}
	added, updated, err := st.ImportRules(ctx, rules, "command line")
	if err != nil {
		exit("cannot import the rules: " + err.Error())
	}
	fmt.Printf("Added %d rules and replaced %d. The collector checks them within 15 seconds.\n", added, updated)
}

// exportRules prints the rules as an alerts.json, enabled ones only, since
// the file format has no way to switch a rule off
func exportRules(ctx context.Context, st *store.Store) {
	migrateOrExit(ctx, st)
	rules, err := st.EnabledRules(ctx)
	if err != nil {
		exit("cannot read the rules: " + err.Error())
	}
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(rules); err != nil {
		exit(err.Error())
	}
}
