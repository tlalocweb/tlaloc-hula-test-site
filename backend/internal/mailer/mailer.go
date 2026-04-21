package mailer

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type Mailer struct {
	domain    string
	apiKey    string
	sender    string
	recipient string
	Verbose   bool
}

func New(domain, apiKey, sender, recipient string) *Mailer {
	return &Mailer{
		domain:    domain,
		apiKey:    apiKey,
		sender:    sender,
		recipient: recipient,
	}
}

func (m *Mailer) SendVerificationCode(toEmail, toName, code string) error {
	subject := "Your Tlaloc verification code"
	body := fmt.Sprintf(
		"Hi %s,\n\nYour verification code is: %s\n\nThis code expires in 10 minutes.\n\n— Tlaloc LLC",
		toName, code,
	)
	return m.send(toEmail, subject, body)
}

func (m *Mailer) SendContactNotification(name, email, message string) error {
	subject := fmt.Sprintf("Contact form: %s", name)
	body := fmt.Sprintf(
		"New contact form submission:\n\nName: %s\nEmail: %s\n\nMessage:\n%s",
		name, email, message,
	)
	return m.send(m.recipient, subject, body)
}

func (m *Mailer) send(to, subject, text string) error {
	apiURL := fmt.Sprintf("https://api.mailgun.net/v3/%s/messages", m.domain)

	form := url.Values{
		"from":    {m.sender},
		"to":      {to},
		"subject": {subject},
		"text":    {text},
	}

	if m.Verbose {
		log.Printf("[mailgun] POST %s", apiURL)
		log.Printf("[mailgun] from=%s to=%s subject=%q", m.sender, to, subject)
	}

	req, err := http.NewRequest("POST", apiURL, strings.NewReader(form.Encode()))
	if err != nil {
		return fmt.Errorf("creating mailgun request: %w", err)
	}

	req.SetBasicAuth("api", m.apiKey)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("mailgun request failed: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	if m.Verbose {
		log.Printf("[mailgun] response status=%d body=%s", resp.StatusCode, string(body))
	}

	if resp.StatusCode >= 400 {
		return fmt.Errorf("mailgun returned status %d: %s", resp.StatusCode, string(body))
	}

	return nil
}
