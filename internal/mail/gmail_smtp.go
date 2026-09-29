package mail

import (
	"context"
	"errors"
	"fmt"
	"net/smtp"
)

var ErrNotConfigured = errors.New("smtp sender is not configured")

type GmailSMTPSender struct {
	host        string
	port        string
	username    string
	appPassword string
	from        string
}

func NewGmailSMTPSender(host, port, username, appPassword, from string) Sender {
	if host == "" || port == "" || username == "" || appPassword == "" || from == "" {
		return disabledSender{}
	}
	return &GmailSMTPSender{host: host, port: port, username: username, appPassword: appPassword, from: from}
}

func (s *GmailSMTPSender) SendActivation(_ context.Context, message ActivationMessage) error {
	auth := smtp.PlainAuth("", s.username, s.appPassword, s.host)
	body := fmt.Sprintf("Subject: Activate your ERBM account\r\nMIME-Version: 1.0\r\nContent-Type: text/plain; charset=UTF-8\r\n\r\nSet your initial password using this link:\r\n%s\r\n", message.ActivationURL)
	return smtp.SendMail(s.host+":"+s.port, auth, s.from, []string{message.To}, []byte(body))
}

type disabledSender struct{}

func (disabledSender) SendActivation(context.Context, ActivationMessage) error {
	return ErrNotConfigured
}
