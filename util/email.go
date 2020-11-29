package util

import (
	"fmt"
	"net/smtp"
	"os"
	"strings"
	"time"

	"github.com/joho/godotenv"
)

func SendEmail(subject string, emailBody string) error {
	err := godotenv.Load()
	if err != nil {
		Log("Error", "Error loading .env file")
		return err
	}
	emailUser := os.Getenv("EMAIL_USER")
	emailPassword := os.Getenv("EMAIL_PASSWORD")
	emailAuth := smtp.PlainAuth("", emailUser, emailPassword, GetConfig().EmailHost)

	mime := "MIME-version: 1.0;\nContent-Type: text/html; charset=\"UTF-8\";\n\n"
	header := "From: " + GetConfig().EmailFrom + "\r\n" +
		"To: " + GetConfig().EmailTo + "\r\n" +
		"Date: " + time.Now().UTC().Format("Mon Jan 02 15:04:05 -0700 2006") + "\r\n" +
		"Subject: " + subject + "\r\n" +
		mime + "\r\n"
	msg := []byte(header + "\n" + emailBody)
	addr := fmt.Sprintf("%s:%s", GetConfig().EmailHost, GetConfig().EmailPort)
	to := strings.Split(GetConfig().EmailTo, ",")

	if err := smtp.SendMail(addr, emailAuth, GetConfig().EmailFrom, to, msg); err != nil {
		return err
	}
	return nil
}
