package email

import (
	"fmt"
	"net/smtp"
	"os"
	"strings"
	"time"
)

func SendEmail(subject string, emailBody string) error {
	emailUser := os.Getenv("EMAIL_USER")
	emailPassword := os.Getenv("EMAIL_PASSWORD")
	emailHost := os.Getenv("EMAIL_HOST")
	emailPort := os.Getenv("EMAIL_PORT")
	emailTo := os.Getenv("ALERT_EMAIL_LIST")
	emailFrom := os.Getenv("EMAIL_FROM")
	emailAuth := smtp.PlainAuth("", emailUser, emailPassword, emailHost)

	mime := "MIME-version: 1.0;\nContent-Type: text/html; charset=\"UTF-8\";\n\n"
	header := "From: " + emailFrom + "\r\n" +
		"To: " + emailTo + "\r\n" +
		"Date: " + time.Now().UTC().Format("Mon Jan 02 15:04:05 -0700 2006") + "\r\n" +
		"Subject: " + subject + "\r\n" +
		mime + "\r\n"
	msg := []byte(header + "\n" + "<pre>" + emailBody + "</pre>")
	addr := fmt.Sprintf("%s:%s", emailHost, emailPort)
	to := strings.Split(emailTo, ",")

	if err := smtp.SendMail(addr, emailAuth, emailFrom, to, msg); err != nil {
		return err
	}
	return nil
}

// GetOpeningEmail return opening email
func GetOpeningEmail(usageType string, usage string, timeDiff string, hostName string, serverTime string) string {
	template := "<p>{usageType} usage is >= {usage}% on {hostName} for {timeDiff} minutes.</p> <br><p>Your attention maybe needed to resolve it</p> <br><p>Server time: {serverTime}</p> <br><br> <p>-- SyMon</p>"
	var replacer = strings.NewReplacer("{usageType}", usageType, "{usage}", usage, "{hostName}", hostName, "{timeDiff}", timeDiff, "{serverTime}", serverTime)
	return replacer.Replace(template)
}

// GetClosingEmail return closing email
func GetClosingEmail(usageType string, timeDiff string, hostName string, serverTime string) string {
	template := "<p>{usageType} usage is now back to normal on {hostName} for {timeDiff} minutes.</p> <br><p>Server time: {serverTime}</p> <br><br> <p>-- SyMon</p>"
	var replacer = strings.NewReplacer("{usageType}", usageType, "{hostName}", hostName, "{timeDiff}", timeDiff, "{serverTime}", serverTime)
	return replacer.Replace(template)
}

// GetDiskUsageOpeningEmail return opening email for disk usage
func GetDiskUsageOpeningEmail(disk string, usage string, serverTime string) string {
	template := "<p>Usage of {disk} is >= {usage}% <br><p>Your attention maybe needed to resolve it</p> <br><p>Server time: {serverTime}</p> <br><br> <p>-- SyMon</p></p>"
	var replacer = strings.NewReplacer("{disk}", disk, "{usage}", usage, "{serverTime}", serverTime)
	return replacer.Replace(template)
}

// GetDiskUsageClosingEmail return closing email for disk usage
func GetDiskUsageClosingEmail(disk string, serverTime string) string {
	template := "<p>Usage of {disk} is went back to normal</p> <br><p>Server time: {serverTime}</p> <br><br> <p>-- SyMon</p></p>"
	var replacer = strings.NewReplacer("{disk}", disk, "{serverTime}", serverTime)
	return replacer.Replace(template)
}
