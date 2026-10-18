package notifications

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const resendEmailsEndpoint = "https://api.resend.com/emails"

type ResendSender struct {
	apiKey   string
	from     string
	endpoint string
	client   *http.Client
}

// resendRejectionError records only safe, structured details from Resend's
// HTTP rejection. The provider's free-form message is intentionally not kept
// because it can contain recipient or request data.
type resendRejectionError struct {
	statusCode int
	name       string
}

func (e *resendRejectionError) Error() string {
	if e.name == "" {
		return fmt.Sprintf("Resend rejected the request (HTTP %d)", e.statusCode)
	}
	return fmt.Sprintf("Resend rejected the request (HTTP %d, provider error %s)", e.statusCode, e.name)
}

type resendInvalidResponseError struct{}

func (*resendInvalidResponseError) Error() string {
	return "Resend returned an invalid response"
}

// safeResendErrorName permits only short identifier-like error names for logs.
func safeResendErrorName(name string) string {
	name = strings.TrimSpace(strings.ToLower(name))
	if name == "" || len(name) > 80 {
		return ""
	}
	for _, r := range name {
		if (r < 'a' || r > 'z') && (r < '0' || r > '9') && r != '_' && r != '-' {
			return ""
		}
	}
	return name
}

func NewResendSender(apiKey, fromEmail string) *ResendSender {
	apiKey = strings.TrimSpace(apiKey)
	fromEmail = strings.TrimSpace(fromEmail)
	if apiKey == "" || fromEmail == "" {
		return nil
	}
	return &ResendSender{
		apiKey:   apiKey,
		from:     fromEmail,
		endpoint: resendEmailsEndpoint,
		client:   &http.Client{Timeout: 10 * time.Second},
	}
}

func (s *ResendSender) Send(ctx context.Context, email Email, idempotencyKey string) (string, error) {
	if s == nil {
		return "", errors.New("Resend sender is not configured")
	}
	payload := struct {
		From    string   `json:"from"`
		To      []string `json:"to"`
		Subject string   `json:"subject"`
		HTML    string   `json:"html"`
		Text    string   `json:"text"`
	}{From: email.From, To: []string{email.To}, Subject: email.Subject, HTML: email.HTML, Text: email.Text}
	body, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("encode Resend request: %w", err)
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, s.endpoint, bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("create Resend request: %w", err)
	}
	request.Header.Set("Authorization", "Bearer "+s.apiKey)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Idempotency-Key", idempotencyKey)

	response, err := s.client.Do(request)
	if err != nil {
		return "", fmt.Errorf("send Resend request: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		var providerError struct {
			Name string `json:"name"`
		}
		// Parse only a small, bounded response. Never log the free-form message,
		// response body, recipient address, or authorization header.
		_ = json.NewDecoder(io.LimitReader(response.Body, 4096)).Decode(&providerError)
		return "", &resendRejectionError{
			statusCode: response.StatusCode,
			name:       safeResendErrorName(providerError.Name),
		}
	}
	var result struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, 16*1024)).Decode(&result); err != nil || strings.TrimSpace(result.ID) == "" {
		return "", &resendInvalidResponseError{}
	}
	return result.ID, nil
}

func (s *ResendSender) endpointForTest(endpoint string, client *http.Client) {
	s.endpoint = endpoint
	s.client = client
}
