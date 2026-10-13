package notifications

import (
	"context"
	"time"
)

type Status string

const (
	StatusSent          Status = "sent"
	StatusFailed        Status = "failed"
	StatusNotConfigured Status = "not_configured"
	StatusSending       Status = "sending"
)

type Claim struct {
	Status           Status
	Detail           string
	RecipientEmail   string
	ProgramName      string
	StartsAt         time.Time
	EndsAt           time.Time
	Location         string
	SupportReference string
	AttemptCount     int
	Claimed          bool
}

type Email struct {
	From    string
	To      string
	Subject string
	HTML    string
	Text    string
}

type Result struct {
	Status    Status `json:"notification_status"`
	Message   string `json:"notification_message"`
	Retryable bool   `json:"notification_retryable"`
}

type Repository interface {
	ClaimApprovalNotification(context.Context, string, bool) (Claim, error)
	CompleteApprovalNotification(context.Context, string, int, bool, string) (bool, error)
}

type Sender interface {
	Send(context.Context, Email, string) (string, error)
}

type ApprovalNotifier interface {
	NotifyApproved(context.Context, string) Result
}
