package adminauth

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const validationTimeout = 2 * time.Second

var (
	ErrInvalidAccessToken = errors.New("invalid access token")
	ErrAuthUnavailable    = errors.New("authentication service unavailable")
)

// Validator asks Supabase Auth to verify the bearer token and returns the
// user UUID from its authenticated response. It never decodes JWT claims.
type Validator struct {
	endpoint string
	apiKey   string
	client   *http.Client
}

func NewValidator(supabaseURL, apiKey string) (*Validator, error) {
	u, err := url.Parse(strings.TrimRight(strings.TrimSpace(supabaseURL), "/"))
	if err != nil || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.Path != "" || (u.Scheme != "https" && !(u.Scheme == "http" && (u.Hostname() == "localhost" || u.Hostname() == "127.0.0.1"))) {
		return nil, errors.New("invalid Supabase URL")
	}
	if apiKey == "" {
		return nil, errors.New("Supabase publishable key is required")
	}
	u.Path = strings.TrimRight(u.Path, "/") + "/auth/v1/user"
	return &Validator{endpoint: u.String(), apiKey: apiKey, client: &http.Client{
		Timeout:       validationTimeout,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}}, nil
}

func (v *Validator) Validate(ctx context.Context, accessToken string) (string, error) {
	if v == nil || v.client == nil {
		return "", ErrAuthUnavailable
	}
	if accessToken == "" {
		return "", ErrInvalidAccessToken
	}
	ctx, cancel := context.WithTimeout(ctx, validationTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, v.endpoint, nil)
	if err != nil {
		return "", ErrAuthUnavailable
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("apikey", v.apiKey)
	resp, err := v.client.Do(req)
	if err != nil {
		return "", ErrAuthUnavailable
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return "", ErrInvalidAccessToken
	}
	if resp.StatusCode != http.StatusOK {
		return "", ErrAuthUnavailable
	}
	var user struct {
		ID string `json:"id"`
	}
	decoder := json.NewDecoder(io.LimitReader(resp.Body, 1<<20))
	if err := decoder.Decode(&user); err != nil || !validUUID(user.ID) {
		return "", ErrAuthUnavailable
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return "", ErrAuthUnavailable
	}
	return strings.ToLower(user.ID), nil
}

func validUUID(s string) bool {
	if len(s) != 36 || s[8] != '-' || s[13] != '-' || s[18] != '-' || s[23] != '-' {
		return false
	}
	for i, c := range s {
		if i == 8 || i == 13 || i == 18 || i == 23 {
			continue
		}
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
			return false
		}
	}
	return true
}

// Protect validates every request at the API boundary and checks the verified
// Auth user ID against the server-only allowlist.
func Protect(next http.Handler, validator interface {
	Validate(context.Context, string) (string, error)
}, allowlist map[string]struct{}) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		parts := strings.Fields(r.Header.Get("Authorization"))
		if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
			writeAuthError(w, http.StatusUnauthorized, "unauthorized")
			return
		}
		if len(allowlist) == 0 || validator == nil {
			writeAuthError(w, http.StatusServiceUnavailable, "admin authentication is temporarily unavailable")
			return
		}
		userID, err := validator.Validate(r.Context(), parts[1])
		if err != nil {
			if errors.Is(err, ErrAuthUnavailable) {
				writeAuthError(w, http.StatusServiceUnavailable, "authentication service is temporarily unavailable")
				return
			}
			writeAuthError(w, http.StatusUnauthorized, "unauthorized")
			return
		}
		if _, ok := allowlist[strings.ToLower(userID)]; !ok {
			writeAuthError(w, http.StatusForbidden, "forbidden")
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), userIDKey{}, strings.ToLower(userID))))
	})
}

// ProtectUser validates that the bearer token belongs to a valid Supabase Auth user.
func ProtectUser(next http.Handler, validator interface {
	Validate(context.Context, string) (string, error)
}) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		parts := strings.Fields(r.Header.Get("Authorization"))
		if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
			writeAuthError(w, http.StatusUnauthorized, "unauthorized")
			return
		}
		if validator == nil {
			writeAuthError(w, http.StatusServiceUnavailable, "authentication service is temporarily unavailable")
			return
		}
		userID, err := validator.Validate(r.Context(), parts[1])
		if err != nil {
			if errors.Is(err, ErrAuthUnavailable) {
				writeAuthError(w, http.StatusServiceUnavailable, "authentication service is temporarily unavailable")
				return
			}
			writeAuthError(w, http.StatusUnauthorized, "unauthorized")
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), userIDKey{}, strings.ToLower(userID))))
	})
}

func writeAuthError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": message})
}

type userIDKey struct{}

func UserID(ctx context.Context) string {
	id, _ := ctx.Value(userIDKey{}).(string)
	return id
}

func UserIDKeyForTest() any {
	return userIDKey{}
}
