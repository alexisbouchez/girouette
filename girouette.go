package main

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"log/slog"
	"net/http"
	"net/mail"
	"os"
	"time"

	"github.com/alexisbouchez/girouette/env"
	"github.com/caddyserver/certmagic"
	"github.com/emersion/go-smtp"
)

type backend struct{}

func (bkd *backend) NewSession(c *smtp.Conn) (smtp.Session, error) {
	fmt.Println("new SMTP session")
	return &session{}, nil
}

type session struct{}

func (s *session) AuthPlain(username, password string) error {
	fmt.Println("AuthPlain", username, password)
	return nil
}

func (s *session) Mail(from string, opts *smtp.MailOptions) error {
	fmt.Println("Mail", from, opts)
	return nil
}

func (s *session) Rcpt(to string, opts *smtp.RcptOptions) error {
	fmt.Println("Rcpt", to, opts)
	return nil
}

func (s *session) Data(r io.Reader) error {
	msg, err := mail.ReadMessage(r)
	if err != nil {
		return err
	}

	sender := msg.Header.Get("From")
	recipients := msg.Header.Get("To")
	subject := msg.Header.Get("Subject")
	body, err := io.ReadAll(msg.Body)
	if err != nil {
		return err
	}

	webhookURL := env.GetVar("WEBHOOK_ENDPOINT", "")
	if webhookURL == "" {
		return fmt.Errorf("missing webhook endpoint in environment variables")
	}

	payload := map[string]string{
		"from":    sender,
		"to":      recipients,
		"subject": subject,
		"body":    string(body),
	}

	jsonPayload, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal JSON payload: %w", err)
	}

	req, err := http.NewRequest("POST", webhookURL, bytes.NewBuffer(jsonPayload))
	if err != nil {
		return fmt.Errorf("failed to create HTTP request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send webhook: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("webhook returned non-200 status: %d", resp.StatusCode)
	}

	fmt.Println("Webhook sent successfully")
	return nil
}

func (s *session) Reset() {
	fmt.Println("Reset")
}

func (s *session) Logout() error {
	fmt.Println("Logout")
	return nil
}

func main() {
	srv := smtp.NewServer(&backend{})

	srv.Addr = env.GetVar("SMTP_ADDR", ":25")
	srv.Domain = env.GetVar("SMTP_DOMAIN", "localhost")
	srv.AllowInsecureAuth = true
	srv.Debug = os.Stdout
	srv.WriteTimeout = 10 * time.Second
	srv.ReadTimeout = 10 * time.Second
	srv.MaxMessageBytes = 1024 * 1024
	srv.MaxRecipients = 50

	// Set the TLS configuration
	if env.GetVar("SMTP_ENABLE_TLS", "false") == "true" {
		certmagic.DefaultACME.Email = env.GetVar("SMTP_ADMIN_EMAIL", "contact@localhost")
		tlsConfig, err := certmagic.TLS([]string{srv.Domain})
		if err != nil {
			log.Fatal("failed to get TLS configuration", "err", err)
		}
		tlsConfig.ClientAuth = tls.RequestClientCert
		tlsConfig.NextProtos = []string{"smtp", "smtps"}

		srv.TLSConfig = tlsConfig
	}

	slog.Info("starting SMTP server", "addr", srv.Addr)
	if srv.TLSConfig != nil {
		if err := srv.ListenAndServeTLS(); err != nil {
			slog.Error("some error happened while serving SMTP server with TLS", "err", err)
			os.Exit(1)
		}
	} else {
		if err := srv.ListenAndServe(); err != nil {
			slog.Error("some error happened while serving SMTP server", "err", err)
			os.Exit(1)
		}
	}
}
