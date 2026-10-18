package notifications

import (
	"context"
	"errors"
	"log"
)

type Service struct {
	repository Repository
	sender     Sender
	fromEmail  string
	logger     interface{ Printf(string, ...any) }
}

func NewService(repository Repository, sender Sender, fromEmail string, logger interface{ Printf(string, ...any) }) *Service {
	if logger == nil {
		logger = log.Default()
	}
	return &Service{repository: repository, sender: sender, fromEmail: fromEmail, logger: logger}
}

func (s *Service) NotifyApproved(ctx context.Context, requestID string) Result {
	configured := s != nil && s.sender != nil && s.fromEmail != ""
	if s == nil || s.repository == nil {
		return Result{Status: StatusFailed, Message: "Approval was saved, but notification status is unavailable. Please contact the requester manually.", Retryable: true}
	}

	claim, err := s.repository.ClaimApprovalNotification(ctx, requestID, configured)
	if err != nil {
		s.logger.Printf("approval notification claim failed (error type %T)", err)
		return Result{Status: StatusFailed, Message: "Approval was saved, but the notification could not be prepared. Please contact the requester manually.", Retryable: true}
	}
	switch claim.Status {
	case StatusSent:
		return sentResult()
	case StatusFailed:
		return failedResult()
	case StatusNotConfigured:
		if claim.Detail == "missing_email" {
			return Result{Status: StatusNotConfigured, Message: "Approval was saved. No email address was provided, so please contact the requester manually.", Retryable: false}
		}
		return Result{Status: StatusNotConfigured, Message: "Approval was saved, but Resend is not configured. Please contact the requester manually.", Retryable: true}
	case StatusSending:
		if !claim.Claimed {
			return Result{Status: StatusSending, Message: "An approval email attempt is already in progress. Please confirm with the requester manually if needed.", Retryable: false}
		}
	default:
		return Result{Status: StatusFailed, Message: "Approval was saved, but the notification status is unknown. Please contact the requester manually.", Retryable: true}
	}

	message, err := BuildApprovalEmail(s.fromEmail, claim)
	if err != nil {
		s.finishFailure(ctx, requestID, claim.AttemptCount, err)
		return failedResult()
	}
	providerID, err := s.sender.Send(ctx, message, "booking-approval/"+requestID)
	if err != nil {
		s.finishFailure(ctx, requestID, claim.AttemptCount, err)
		return failedResult()
	}

	saved, err := s.repository.CompleteApprovalNotification(ctx, requestID, claim.AttemptCount, true, providerID)
	if err != nil {
		s.logger.Printf("approval notification status save failed (error type %T)", err)
		return Result{Status: StatusSent, Message: "Resend accepted the approval email, but its status could not be saved. Delivery is not confirmed; contact the requester manually before retrying.", Retryable: false}
	}
	if !saved {
		return Result{Status: StatusSending, Message: "Resend accepted the approval email, but another attempt is updating its status. Delivery is not confirmed; contact the requester manually if needed.", Retryable: false}
	}
	return sentResult()
}

func (s *Service) finishFailure(ctx context.Context, requestID string, attempt int, cause error) {
	var invalidResponse *resendInvalidResponseError
	var rejection *resendRejectionError
	if errors.As(cause, &invalidResponse) {
		s.logger.Printf("approval notification provider failed (provider=resend failure=invalid_response)")
	} else if errors.As(cause, &rejection) {
		if rejection.name == "" {
			s.logger.Printf("approval notification provider failed (provider=resend http_status=%d)", rejection.statusCode)
		} else {
			s.logger.Printf("approval notification provider failed (provider=resend http_status=%d provider_error=%s)", rejection.statusCode, rejection.name)
		}
	} else {
		// Preserve type-only diagnostics for non-Resend errors to avoid exposing
		// arbitrary error strings that may contain sensitive request data.
		s.logger.Printf("approval notification provider failed (error type %T)", cause)
	}
	if _, err := s.repository.CompleteApprovalNotification(ctx, requestID, attempt, false, ""); err != nil {
		s.logger.Printf("approval notification failure status save failed (error type %T)", err)
	}
}

func sentResult() Result {
	return Result{Status: StatusSent, Message: "Resend accepted the approval email. Delivery is not confirmed; please also contact the requester personally.", Retryable: false}
}

func failedResult() Result {
	return Result{Status: StatusFailed, Message: "Approval was saved, but the approval email failed. Please contact the requester manually.", Retryable: true}
}
