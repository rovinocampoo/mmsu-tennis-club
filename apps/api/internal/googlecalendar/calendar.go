package googlecalendar

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/rovinocampoo/mmsu-tennis-club/apps/api/internal/training"
)

const tokenURL = "https://oauth2.googleapis.com/token"
const apiURL = "https://www.googleapis.com/calendar/v3"

type account struct {
	Email    string `json:"client_email"`
	Key      string `json:"private_key"`
	TokenURI string `json:"token_uri"`
}
type tokenResult struct {
	AccessToken string `json:"access_token"`
	ExpiresIn   int64  `json:"expires_in"`
}
type Client struct {
	file, calendarID string
	http             *http.Client
	mu               sync.Mutex
	acct             *account
	key              *rsa.PrivateKey
	token            string
	expiry           time.Time
	loadErr          error
}

func New(file, calendarID string) *Client {
	return &Client{file: strings.TrimSpace(file), calendarID: strings.TrimSpace(calendarID), http: &http.Client{Timeout: 8 * time.Second}}
}
func (c *Client) IsConfigured() bool { return c != nil && c.file != "" && c.calendarID != "" }
func eventID(id string) string       { return "mmsu" + strings.ReplaceAll(strings.ToLower(id), "-", "") }

type eventTime struct {
	DateTime string `json:"dateTime"`
	TimeZone string `json:"timeZone"`
}
type eventPayload struct {
	ID          string    `json:"id,omitempty"`
	Summary     string    `json:"summary"`
	Description string    `json:"description"`
	Location    string    `json:"location"`
	Start       eventTime `json:"start"`
	End         eventTime `json:"end"`
}

func (c *Client) Sync(ctx context.Context, e training.CalendarEvent) error {
	if !c.IsConfigured() {
		return errors.New("Google Calendar is not configured")
	}
	if e.RequestID == "" || e.RequestReference == "" || e.ProgramName == "" || !e.StartsAt.Before(e.EndsAt) {
		return errors.New("booking calendar data is incomplete")
	}
	tok, err := c.getToken(ctx)
	if err != nil {
		return err
	}
	pht := time.FixedZone("Asia/Manila", 8*60*60)
	p := eventPayload{ID: eventID(e.RequestID), Summary: e.ParticipantName + " — " + e.ProgramName, Description: fmt.Sprintf("MMSU Tennis Club booking\nReference: %s\nParticipant: %s", e.RequestReference, e.ParticipantName), Location: e.Location, Start: eventTime{e.StartsAt.In(pht).Format(time.RFC3339), "Asia/Manila"}, End: eventTime{e.EndsAt.In(pht).Format(time.RFC3339), "Asia/Manila"}}
	body, _ := json.Marshal(p)
	endpoint := apiURL + "/calendars/" + url.PathEscape(c.calendarID) + "/events"
	status, err := c.request(ctx, http.MethodPost, endpoint, tok, body)
	if err != nil {
		return err
	}
	if status == http.StatusConflict {
		p.ID = ""
		body, _ = json.Marshal(p)
		status, err = c.request(ctx, http.MethodPatch, endpoint+"/"+url.PathEscape(eventID(e.RequestID)), tok, body)
		if err != nil {
			return err
		}
	}
	if status < 200 || status >= 300 {
		return fmt.Errorf("Google Calendar returned HTTP %d", status)
	}
	return nil
}
func (c *Client) getToken(ctx context.Context) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.token != "" && time.Until(c.expiry) > time.Minute {
		return c.token, nil
	}
	if c.loadErr != nil {
		return "", c.loadErr
	}
	if c.acct == nil {
		raw, err := os.ReadFile(c.file)
		if err != nil {
			c.loadErr = fmt.Errorf("read service-account secret file: %w", err)
			return "", c.loadErr
		}
		var a account
		if err = json.Unmarshal(raw, &a); err != nil || a.Email == "" || a.Key == "" {
			c.loadErr = errors.New("service-account JSON is invalid or missing required fields")
			return "", c.loadErr
		}
		b, _ := pem.Decode([]byte(a.Key))
		if b == nil {
			c.loadErr = errors.New("service-account private key is invalid")
			return "", c.loadErr
		}
		k, err := x509.ParsePKCS8PrivateKey(b.Bytes)
		if err != nil {
			k, err = x509.ParsePKCS1PrivateKey(b.Bytes)
		}
		if err != nil {
			c.loadErr = errors.New("service-account private key could not be parsed")
			return "", c.loadErr
		}
		rsaKey, ok := k.(*rsa.PrivateKey)
		if !ok {
			c.loadErr = errors.New("service-account private key is not RSA")
			return "", c.loadErr
		}
		c.acct = &a
		c.key = rsaKey
	}
	target := c.acct.TokenURI
	if target == "" {
		target = tokenURL
	}
	u, err := url.Parse(target)
	if err != nil || u.Scheme != "https" || u.Host == "" {
		return "", errors.New("token_uri must be HTTPS")
	}
	now := time.Now()
	h, _ := json.Marshal(map[string]string{"alg": "RS256", "typ": "JWT"})
	cl, _ := json.Marshal(map[string]any{"iss": c.acct.Email, "scope": "https://www.googleapis.com/auth/calendar.events", "aud": target, "iat": now.Unix(), "exp": now.Add(time.Hour).Unix()})
	unsigned := base64.RawURLEncoding.EncodeToString(h) + "." + base64.RawURLEncoding.EncodeToString(cl)
	sum := sha256.Sum256([]byte(unsigned))
	sig, err := rsa.SignPKCS1v15(rand.Reader, c.key, crypto.SHA256, sum[:])
	if err != nil {
		return "", err
	}
	assertion := unsigned + "." + base64.RawURLEncoding.EncodeToString(sig)
	form := url.Values{"grant_type": {"urn:ietf:params:oauth:grant-type:jwt-bearer"}, "assertion": {assertion}}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, target, strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := c.http.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("Google token endpoint returned HTTP %d", resp.StatusCode)
	}
	var tr tokenResult
	if json.Unmarshal(raw, &tr) != nil || tr.AccessToken == "" || tr.ExpiresIn < 1 {
		return "", errors.New("Google token response is invalid")
	}
	c.token = tr.AccessToken
	c.expiry = now.Add(time.Duration(tr.ExpiresIn) * time.Second)
	return c.token, nil
}
func (c *Client) request(ctx context.Context, method, endpoint, token string, body []byte) (int, error) {
	req, err := http.NewRequestWithContext(ctx, method, endpoint, strings.NewReader(string(body)))
	if err != nil {
		return 0, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode == http.StatusConflict || resp.StatusCode == http.StatusNotFound {
		return resp.StatusCode, nil
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return resp.StatusCode, fmt.Errorf("Google Calendar API returned HTTP %d", resp.StatusCode)
	}
	return resp.StatusCode, nil
}
