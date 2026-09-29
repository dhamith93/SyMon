package alerts

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"strings"

	"github.com/dhamith93/SyMon/internal/monitor"
)

// AllHosts in a rule's Servers means every registered host
const AllHosts = "*"

var thresholdOps = map[string]bool{">": true, "<": true, ">=": true, "<=": true, "==": true, "!=": true}

// LoadRules reads an alerts.json file. Any mistake in it is an error, so a
// typo cannot quietly switch alerts off.
func LoadRules(path string) ([]AlertConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	rules, err := ParseRules(data)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return rules, nil
}

// ParseRules reads a JSON list of rules. Unknown fields are errors, since
// they are almost always a misspelled one.
func ParseRules(data []byte) ([]AlertConfig, error) {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var rules []AlertConfig
	if err := decoder.Decode(&rules); err != nil {
		return nil, jsonError(data, err)
	}
	names := map[string]bool{}
	for i := range rules {
		rule := &rules[i]
		if err := rule.Validate(); err != nil {
			return nil, fmt.Errorf("rule %d %q: %w", i+1, rule.Name, err)
		}
		if names[rule.Name] {
			return nil, fmt.Errorf("rule %d: there is already a rule named %q", i+1, rule.Name)
		}
		names[rule.Name] = true
	}
	return rules, nil
}

// jsonError adds the line and column to a syntax error
func jsonError(data []byte, err error) error {
	var syntax *json.SyntaxError
	var wrongType *json.UnmarshalTypeError
	offset := int64(-1)
	switch {
	case errors.As(err, &syntax):
		offset = syntax.Offset
	case errors.As(err, &wrongType):
		offset = wrongType.Offset
	}
	if offset < 0 {
		return err
	}
	// the offset counts the byte that went wrong
	before := data[:min(int(offset), len(data))]
	line := bytes.Count(before, []byte("\n")) + 1
	column := max(len(before)-bytes.LastIndexByte(before, '\n')-1, 1)
	return fmt.Errorf("line %d, column %d: %w", line, column, err)
}

// Validate checks that a rule has what its metric needs
func (rule *AlertConfig) Validate() error {
	rule.Name = strings.TrimSpace(rule.Name)
	switch {
	case rule.Name == "":
		return errors.New("a rule needs a Name")
	case len(rule.Name) > 100:
		return errors.New("a Name can be at most 100 characters")
	case rule.TriggerIntveral < 0:
		return errors.New("TriggerIntveral cannot be negative")
	}

	if rule.IsCustom {
		if strings.TrimSpace(rule.MetricName) == "" {
			return errors.New("a custom metric rule needs MetricName, the custom metric's name")
		}
		return firstError(rule.checkServers(), rule.checkOp())
	}

	switch rule.MetricName {
	case monitor.PROC_USAGE, monitor.MEMORY, monitor.SWAP:
		return firstError(rule.checkServers(), rule.checkOp())
	case monitor.DISKS, monitor.DISK_FORECAST:
		if strings.TrimSpace(rule.Disk) == "" {
			return errors.New("a disk rule needs Disk, the device like /dev/sda1")
		}
		return firstError(rule.checkServers(), rule.checkOp())
	case monitor.SERVICES:
		if strings.TrimSpace(rule.Service) == "" {
			return errors.New("a service rule needs Service, a name from the agent's service list")
		}
		if rule.Op != "active" && rule.Op != "inactive" {
			return errors.New(`a service rule needs Op "inactive" (alert when it stops) or "active" (alert when it runs)`)
		}
		return rule.checkServers()
	case monitor.PING:
		return rule.checkServers()
	case monitor.ENDPOINT:
		return rule.checkEndpoint()
	case "":
		return errors.New("a rule needs MetricName")
	}
	return fmt.Errorf("unknown MetricName %q, it can be procUsage, memory, swap, disks, disk_forecast, services, ping, endpoint, or a custom metric's name with IsCustom", rule.MetricName)
}

func (rule *AlertConfig) checkServers() error {
	if len(rule.Servers) == 0 {
		return fmt.Errorf(`a rule needs Servers, host names or "%s" for all hosts`, AllHosts)
	}
	for _, server := range rule.Servers {
		if strings.TrimSpace(server) == "" {
			return errors.New("Servers has an empty host name")
		}
	}
	return nil
}

func (rule *AlertConfig) checkOp() error {
	if !thresholdOps[rule.Op] {
		return fmt.Errorf("unknown Op %q, it can be >, <, >=, <=, == or !=", rule.Op)
	}
	return nil
}

func (rule *AlertConfig) checkEndpoint() error {
	target, err := url.Parse(rule.Endpoint)
	if err != nil || (target.Scheme != "http" && target.Scheme != "https") || target.Host == "" {
		return errors.New("an endpoint rule needs Endpoint, an http:// or https:// URL")
	}
	switch strings.ToUpper(strings.TrimSpace(rule.Method)) {
	case "", "GET", "HEAD", "POST":
	default:
		return fmt.Errorf("unknown Method %q, it can be GET, HEAD or POST", rule.Method)
	}
	if rule.ExpectedHTTPCode != 0 && (rule.ExpectedHTTPCode < 100 || rule.ExpectedHTTPCode > 599) {
		return errors.New("ExpectedHTTPCode has to be an HTTP status code, like 200")
	}
	return nil
}

func firstError(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			return err
		}
	}
	return nil
}
