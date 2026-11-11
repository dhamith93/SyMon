package snmp

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net"
	"os"
	"regexp"
	"strconv"
	"strings"
	"syscall"

	"github.com/dhamith93/SyMon/internal/strictjson"
)

// The devices file, /etc/symon/snmp.json by default. It holds community
// strings, so the agent only reads it when nobody else can.

const (
	DefaultPort   = 161
	maxFileSize   = 256 << 10
	maxDevices    = 200
	maxNameLength = 64
	maxKindLength = 32
	minInterval   = 10
	maxInterval   = 3600
)

// The profiles a device can read, sets of well-known OIDs. The system group
// is always read.
const (
	ProfileInterfaces = "interfaces"
	ProfileHost       = "host"
	ProfileUPS        = "ups"
)

var profiles = map[string]bool{ProfileInterfaces: true, ProfileHost: true, ProfileUPS: true}

// oidPattern is a dotted OID, with or without the leading dot
var oidPattern = regexp.MustCompile(`^\.?[0-9]+(\.[0-9]+)+$`)

// Device is one entry of the devices file
type Device struct {
	// Name is the device on the dashboard and in alert rules
	Name string
	// Address is host or host:port, port 161 unless given
	Address string
	// Community is the SNMP v2c community, a password in all but name
	Community string
	// Kind is shown on the dashboard. Worked out from the profiles unless set.
	Kind string `json:",omitempty"`
	// IntervalSeconds polls this device on its own schedule instead of the
	// agent's SYMON_SNMP_INTERVAL_SECONDS
	IntervalSeconds int `json:",omitempty"`
	// Profiles are the sets of OIDs to read
	Profiles []string `json:",omitempty"`
	// Interfaces keeps only these ports, by name. Empty keeps the physical
	// ports and drops VLANs and the like.
	Interfaces []string `json:",omitempty"`
	// Metrics are extra OIDs to read
	Metrics []Metric `json:",omitempty"`
}

// Metric is one OID the user asked for on top of the profiles
type Metric struct {
	Name string
	OID  string
	Unit string `json:",omitempty"`
	// Counter turns an ever growing number into a per second rate
	Counter bool `json:",omitempty"`
	// Scale multiplies the value, like 0.1 for tenths of a degree. 0 means 1.
	Scale float64 `json:",omitempty"`
}

// Load reads the devices file. No file means no devices. Any mistake in the
// file is an error, and so is a file other users can read, because it holds
// the communities.
func Load(path string) ([]Device, error) {
	if path == "" {
		return nil, nil
	}
	file, err := os.Open(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer file.Close()

	info, err := file.Stat()
	if err != nil {
		return nil, err
	}
	if err := checkPrivate(info); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	data, err := io.ReadAll(io.LimitReader(file, maxFileSize+1))
	if err != nil {
		return nil, err
	}
	if len(data) > maxFileSize {
		return nil, fmt.Errorf("%s: too big for a devices file", path)
	}
	devices, err := Parse(data)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return devices, nil
}

// checkPrivate is stricter than the tasks file's check: the communities
// must not be readable by anyone but the owner
func checkPrivate(info fs.FileInfo) error {
	if !info.Mode().IsRegular() {
		return errors.New("not a regular file")
	}
	if info.Mode().Perm()&0o077 != 0 {
		return errors.New("group or others can read it, run chmod 600 on it")
	}
	if stat, ok := info.Sys().(*syscall.Stat_t); ok && stat.Uid != 0 && int(stat.Uid) != os.Geteuid() {
		return errors.New("it has to belong to root or to the user the agent runs as")
	}
	return nil
}

// Parse reads a JSON list of devices. Unknown fields are errors.
func Parse(data []byte) ([]Device, error) {
	var devices []Device
	if err := strictjson.Unmarshal(data, &devices); err != nil {
		return nil, err
	}
	if err := Validate(devices); err != nil {
		return nil, err
	}
	return devices, nil
}

// Validate checks the devices and tidies names and OIDs
func Validate(devices []Device) error {
	if len(devices) > maxDevices {
		return fmt.Errorf("at most %d devices", maxDevices)
	}
	seen := map[string]bool{}
	for i := range devices {
		device := &devices[i]
		device.Name = strings.TrimSpace(device.Name)
		if device.Name == "" {
			return fmt.Errorf("device %d has no Name", i+1)
		}
		if seen[device.Name] {
			return fmt.Errorf("there are two devices named %q", device.Name)
		}
		seen[device.Name] = true
		if err := device.validate(); err != nil {
			return fmt.Errorf("device %q: %w", device.Name, err)
		}
	}
	return nil
}

func (device *Device) validate() error {
	if len(device.Name) > maxNameLength {
		return fmt.Errorf("the Name can be at most %d characters", maxNameLength)
	}
	if strings.Contains(device.Name, "/") {
		return errors.New("the Name cannot contain /, which separates a device from its port in alerts")
	}
	device.Address = strings.TrimSpace(device.Address)
	if _, _, err := hostPort(device.Address); err != nil {
		return err
	}
	if device.Community == "" {
		return errors.New("it needs a Community")
	}
	device.Kind = strings.TrimSpace(device.Kind)
	if len(device.Kind) > maxKindLength {
		return fmt.Errorf("the Kind can be at most %d characters", maxKindLength)
	}
	if device.IntervalSeconds != 0 && (device.IntervalSeconds < minInterval || device.IntervalSeconds > maxInterval) {
		return fmt.Errorf("IntervalSeconds has to be between %d and %d", minInterval, maxInterval)
	}
	seen := map[string]bool{}
	for i, profile := range device.Profiles {
		device.Profiles[i] = strings.TrimSpace(profile)
		if !profiles[device.Profiles[i]] {
			return fmt.Errorf("unknown profile %q, it can be %s, %s or %s", profile, ProfileInterfaces, ProfileHost, ProfileUPS)
		}
		if seen[device.Profiles[i]] {
			return fmt.Errorf("the profile %q is listed twice", profile)
		}
		seen[device.Profiles[i]] = true
	}
	for i, name := range device.Interfaces {
		device.Interfaces[i] = strings.TrimSpace(name)
		if device.Interfaces[i] == "" {
			return errors.New("a name in Interfaces is empty")
		}
	}
	if len(device.Profiles) == 0 && len(device.Metrics) == 0 {
		return errors.New("it needs at least one profile or metric")
	}
	names := map[string]bool{}
	for i := range device.Metrics {
		metric := &device.Metrics[i]
		if err := metric.validate(); err != nil {
			return err
		}
		if names[metric.Name] {
			return fmt.Errorf("there are two metrics named %q", metric.Name)
		}
		names[metric.Name] = true
	}
	return nil
}

func (metric *Metric) validate() error {
	metric.Name = strings.TrimSpace(metric.Name)
	switch {
	case metric.Name == "":
		return errors.New("a metric has no Name")
	case len(metric.Name) > maxNameLength:
		return fmt.Errorf("metric %q: the Name can be at most %d characters", metric.Name, maxNameLength)
	case builtinMetrics[metric.Name]:
		return fmt.Errorf("metric %q: that name is taken by a profile", metric.Name)
	}
	metric.OID = strings.TrimSpace(metric.OID)
	if !oidPattern.MatchString(metric.OID) {
		return fmt.Errorf("metric %q: %q is not an OID like 1.3.6.1.2.1.1.3.0", metric.Name, metric.OID)
	}
	metric.OID = strings.TrimPrefix(metric.OID, ".")
	if metric.Scale < 0 {
		return fmt.Errorf("metric %q: Scale cannot be negative", metric.Name)
	}
	return nil
}

// hostPort splits an address into host and port, 161 unless given
func hostPort(address string) (string, uint16, error) {
	if address == "" {
		return "", 0, errors.New("it needs an Address")
	}
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		// no port, or a bare IPv6 address
		return strings.Trim(address, "[]"), DefaultPort, nil
	}
	number, err := strconv.Atoi(port)
	if err != nil || number < 1 || number > 65535 {
		return "", 0, fmt.Errorf("%q is not a port", port)
	}
	if host == "" {
		return "", 0, fmt.Errorf("%q has no host", address)
	}
	return host, uint16(number), nil
}

// Find returns the device with that name
func Find(devices []Device, name string) (Device, bool) {
	for _, device := range devices {
		if device.Name == name {
			return device, true
		}
	}
	return Device{}, false
}

// kindOf is the device's kind for the dashboard: what the file says, else
// the most telling of its profiles
func kindOf(device Device) string {
	if device.Kind != "" {
		return device.Kind
	}
	has := map[string]bool{}
	for _, profile := range device.Profiles {
		has[profile] = true
	}
	switch {
	case has[ProfileUPS]:
		return KindUPS
	case has[ProfileInterfaces]:
		return KindSwitch
	case has[ProfileHost]:
		return KindServer
	}
	return KindOther
}
