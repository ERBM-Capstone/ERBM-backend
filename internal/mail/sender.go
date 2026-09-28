// Package mail provides delivery for account-activation messages.
package mail

import "context"

type ActivationMessage struct {
	To            string
	ActivationURL string
}

// Sender is deliberately small so production SMTP can be replaced in tests.
type Sender interface {
	SendActivation(ctx context.Context, message ActivationMessage) error
}
