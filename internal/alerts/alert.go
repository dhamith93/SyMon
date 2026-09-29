package alerts

import (
	"strings"

	"github.com/dhamith93/SyMon/internal/monitor"
)

type AlertConfig struct {
	Name              string
	Description       string
	MetricName        string
	IsCustom          bool
	Op                string
	Template          string
	WarnThreshold     int
	CriticalThreshold int
	TriggerIntveral   int
	Servers           []string
	Endpoint          string
	ExpectedHTTPCode  int
	Method            string
	CustomCACert      string
	POSTContentType   string
	POSTBody          string
	Disk              string
	Service           string
	Email             bool
	Pagerduty         bool
	Slack             bool
	SlackChannel      string
	// CertWarnDays and CertCriticalDays alert on an HTTPS endpoint's
	// certificate that expires in fewer days. Left out they are 14 and 3,
	// and 0 switches a level off.
	CertWarnDays     *int `json:",omitempty"`
	CertCriticalDays *int `json:",omitempty"`
}

// certificate alert levels when a rule does not set them
const (
	defaultCertWarnDays     = 14
	defaultCertCriticalDays = 3
)

// CertDays are the days left at which an HTTPS endpoint rule's certificate
// alert turns warning and critical. Both are 0 when there is none, like for
// a plain HTTP endpoint.
func (rule *AlertConfig) CertDays() (warn int, critical int) {
	if rule.MetricName != monitor.ENDPOINT || !strings.HasPrefix(strings.ToLower(rule.Endpoint), "https://") {
		return 0, 0
	}
	warn, critical = defaultCertWarnDays, defaultCertCriticalDays
	if rule.CertWarnDays != nil {
		warn = *rule.CertWarnDays
	}
	if rule.CertCriticalDays != nil {
		critical = *rule.CertCriticalDays
	}
	return warn, critical
}

type Alert struct {
	ServerName        string
	Name              string
	Template          string
	MetricName        string
	Op                string
	Timestamp         string
	Value             float32
	WarnThreshold     int
	CriticalThreshold int
	TriggerIntveral   int
}
