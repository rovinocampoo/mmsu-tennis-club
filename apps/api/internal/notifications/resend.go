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
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4096))
		return "", errors.New("Resend rejected the request")
	}
	var result struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, 16*1024)).Decode(&result); err != nil || strings.TrimSpace(result.ID) == "" {
		return "", errors.New("Resend returned an invalid response")
	}
	return result.ID, nil
}

func (s *ResendSender) endpointForTest(endpoint string, client *http.Client) {
	s.endpoint = endpoint
	s.client = client
}
