package notify

import (
	"bytes"
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"mime"
	"mime/quotedprintable"
	"net"
	"net/mail"
	"net/smtp"
	"strconv"
	"strings"
	"time"
)

// emailSender sends a plain text email over SMTP. Port 465 uses TLS from
// the start, other ports switch to TLS when the server offers it. A
// password is only ever sent over TLS, or to the same machine.
type emailSender struct {
	Host     string   `json:"host"`
	Port     int      `json:"port"`
	User     string   `json:"user"`
	Password string   `json:"password"`
	From     string   `json:"from"`
	To       []string `json:"to"`
}

func (e *emailSender) validate() error {
	e.Host = strings.TrimSpace(e.Host)
	if e.Port == 0 {
		e.Port = 587
	}
	switch {
	case e.Host == "":
		return errors.New("an email channel needs the SMTP server's host")
	case e.Port < 1 || e.Port > 65535:
		return errors.New("the SMTP port goes from 1 to 65535")
	case e.User != "" && e.Password == "":
		return errors.New("a user needs a password")
	case len(e.To) == 0:
		return errors.New("an email channel needs at least one address to send to")
	}
	if _, err := mail.ParseAddress(e.From); err != nil {
		return fmt.Errorf("the from address %q: %w", e.From, err)
	}
	for _, to := range e.To {
		if _, err := mail.ParseAddress(to); err != nil {
			return fmt.Errorf("the address %q: %w", to, err)
		}
	}
	return nil
}

func (e *emailSender) Send(ctx context.Context, msg Message, _ string) (string, error) {
	data, err := e.compose(msg, time.Now())
	if err != nil {
		return "", err
	}
	return "", e.deliver(ctx, data)
}

// compose writes the whole email, headers and body
func (e *emailSender) compose(msg Message, now time.Time) ([]byte, error) {
	from, _ := mail.ParseAddress(e.From)
	to := make([]string, 0, len(e.To))
	for _, address := range e.To {
		parsed, _ := mail.ParseAddress(address)
		to = append(to, parsed.String())
	}
	var data bytes.Buffer
	header := func(name string, value string) {
		// a line break in a value would start a header of its own
		value = strings.NewReplacer("\r", " ", "\n", " ").Replace(value)
		fmt.Fprintf(&data, "%s: %s\r\n", name, value)
	}
	header("From", from.String())
	header("To", strings.Join(to, ", "))
	subject := strings.NewReplacer("\r", " ", "\n", " ").Replace(msg.Subject)
	header("Subject", mime.QEncoding.Encode("utf-8", subject))
	header("Date", now.Format(time.RFC1123Z))
	header("MIME-Version", "1.0")
	header("Content-Type", "text/plain; charset=utf-8")
	header("Content-Transfer-Encoding", "quoted-printable")
	data.WriteString("\r\n")

	body := msg.Body
	if msg.Link != "" {
		body += "\n\n" + msg.Link
	}
	body += "\n\n-- SyMon\n"
	writer := quotedprintable.NewWriter(&data)
	if _, err := writer.Write([]byte(strings.ReplaceAll(body, "\n", "\r\n"))); err != nil {
		return nil, err
	}
	if err := writer.Close(); err != nil {
		return nil, err
	}
	return data.Bytes(), nil
}

func (e *emailSender) deliver(ctx context.Context, data []byte) error {
	ctx, cancel := context.WithTimeout(ctx, sendTimeout)
	defer cancel()
	address := net.JoinHostPort(e.Host, strconv.Itoa(e.Port))
	var dialer net.Dialer
	conn, err := dialer.DialContext(ctx, "tcp", address)
	if err != nil {
		return err
	}
	defer conn.Close()
	if deadline, ok := ctx.Deadline(); ok {
		conn.SetDeadline(deadline)
	}
	tlsConfig := &tls.Config{ServerName: e.Host, MinVersion: tls.VersionTLS12}
	if e.Port == 465 {
		conn = tls.Client(conn, tlsConfig)
	}

	client, err := smtp.NewClient(conn, e.Host)
	if err != nil {
		return err
	}
	defer client.Close()
	if ok, _ := client.Extension("STARTTLS"); ok && e.Port != 465 {
		if err := client.StartTLS(tlsConfig); err != nil {
			return err
		}
	}
	if e.User != "" {
		// PlainAuth refuses to send the password without TLS, unless the
		// server is on this machine
		if err := client.Auth(smtp.PlainAuth("", e.User, e.Password, e.Host)); err != nil {
			return err
		}
	}
	from, _ := mail.ParseAddress(e.From)
	if err := client.Mail(from.Address); err != nil {
		return err
	}
	for _, to := range e.To {
		parsed, _ := mail.ParseAddress(to)
		if err := client.Rcpt(parsed.Address); err != nil {
			return err
		}
	}
	writer, err := client.Data()
	if err != nil {
		return err
	}
	if _, err := writer.Write(data); err != nil {
		return err
	}
	if err := writer.Close(); err != nil {
		return err
	}
	return client.Quit()
}
