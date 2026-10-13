package adminauth

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

const testAdminID = "11111111-1111-4111-8111-111111111111"
const testOtherID = "22222222-2222-4222-8222-222222222222"

func TestProtectAllowsVerifiedAllowlistedUser(t *testing.T) {
	var sawHeaders bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sawHeaders = r.URL.Path == "/auth/v1/user" && r.Header.Get("Authorization") == "Bearer valid-token" && r.Header.Get("apikey") == "public-key"
		fmt.Fprintf(w, `{"id":%q,"email":"admin@example.com"}`, testAdminID)
	}))
	defer server.Close()
	validator, err := NewValidator(server.URL, "public-key")
	if err != nil {
		t.Fatal(err)
	}
	called := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = UserID(r.Context()) == testAdminID
		w.WriteHeader(http.StatusNoContent)
	})
	response := protectedRequest(Protect(next, validator, map[string]struct{}{testAdminID: {}}), "Bearer valid-token")
	if response.Code != http.StatusNoContent || !called || !sawHeaders {
		t.Fatalf("response=%d called=%t headers=%t", response.Code, called, sawHeaders)
	}
}

func TestProtectDeniesVerifiedNonAllowlistedUser(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { fmt.Fprintf(w, `{"id":%q}`, testOtherID) }))
	defer server.Close()
	validator, _ := NewValidator(server.URL, "public-key")
	response := protectedRequest(Protect(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { t.Fatal("handler ran") }), validator, map[string]struct{}{testAdminID: {}}), "Bearer valid-token")
	if response.Code != http.StatusForbidden {
		t.Fatalf("status=%d", response.Code)
	}
}

func TestProtectUserAllowsAnyVerifiedUser(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprintf(w, `{"id":%q,"email":"coach@example.com"}`, testOtherID)
	}))
	defer server.Close()
	validator, err := NewValidator(server.URL, "public-key")
	if err != nil {
		t.Fatal(err)
	}
	called := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = UserID(r.Context()) == testOtherID
		w.WriteHeader(http.StatusNoContent)
	})
	response := protectedRequest(ProtectUser(next, validator), "Bearer valid-token")
	if response.Code != http.StatusNoContent || !called {
		t.Fatalf("response=%d called=%t", response.Code, called)
	}
}

func TestProtectNormalizesVerifiedAuthUUIDBeforeAllowlistCheck(t *testing.T) {
	const mixedCaseID = "ABCDEFAB-CDEF-4ABC-8DEF-ABCDEFABCDEF"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { fmt.Fprintf(w, `{"id":%q}`, mixedCaseID) }))
	defer server.Close()
	validator, _ := NewValidator(server.URL, "public-key")
	called := false
	handler := Protect(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = UserID(r.Context()) == strings.ToLower(mixedCaseID)
		w.WriteHeader(http.StatusNoContent)
	}), validator, map[string]struct{}{strings.ToLower(mixedCaseID): {}})
	response := protectedRequest(handler, "Bearer valid-token")
	if response.Code != http.StatusNoContent || !called {
		t.Fatalf("status=%d normalized user context=%t", response.Code, called)
	}
}

func TestProtectSeparatesInvalidCredentialsFromAuthServiceFailures(t *testing.T) {
	for _, tc := range []struct {
		name       string
		status     int
		body       string
		wantStatus int
	}{
		{name: "invalid token", status: 401, body: `{"message":"invalid"}`},
		{name: "expired token", status: 401, body: `{"message":"expired"}`},
		{name: "unexpected response", status: 200, body: `{"id":"not-a-uuid"}`, wantStatus: http.StatusServiceUnavailable},
		{name: "malformed response", status: 200, body: `not-json`, wantStatus: http.StatusServiceUnavailable},
		{name: "Auth rate limited", status: http.StatusTooManyRequests, body: `{"message":"private upstream detail"}`, wantStatus: http.StatusServiceUnavailable},
		{name: "Auth server error", status: http.StatusBadGateway, body: `{"message":"private upstream detail"}`, wantStatus: http.StatusServiceUnavailable},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(tc.body))
			}))
			defer server.Close()
			validator, _ := NewValidator(server.URL, "public-key")
			response := protectedRequest(Protect(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { t.Fatal("handler ran") }), validator, map[string]struct{}{testAdminID: {}}), "Bearer token")
			wantStatus := tc.wantStatus
			if wantStatus == 0 {
				wantStatus = http.StatusUnauthorized
			}
			if response.Code != wantStatus {
				t.Fatalf("status=%d want=%d", response.Code, wantStatus)
			}
			if strings.Contains(response.Body.String(), "private upstream detail") {
				t.Fatalf("response exposed upstream details: %s", response.Body.String())
			}
		})
	}
}

func TestProtectDistinguishesMissingAuthorizationFromVerifierMisconfiguration(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { requests++; w.WriteHeader(http.StatusOK) }))
	defer server.Close()
	validator, _ := NewValidator(server.URL, "public-key")
	handler := Protect(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { t.Fatal("handler ran") }), validator, map[string]struct{}{testAdminID: {}})
	if got := protectedRequest(handler, "").Code; got != http.StatusUnauthorized {
		t.Fatalf("missing auth status=%d", got)
	}
	if got := protectedRequest(Protect(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { t.Fatal("handler ran") }), nil, map[string]struct{}{testAdminID: {}}), "Bearer token").Code; got != http.StatusServiceUnavailable {
		t.Fatalf("unavailable validator status=%d", got)
	}
	if requests != 0 {
		t.Fatalf("Auth called %d times", requests)
	}
}

func TestProtectReportsAuthTimeoutAsTemporaryServiceFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { <-r.Context().Done() }))
	defer server.Close()
	validator, _ := NewValidator(server.URL, "public-key")
	validator.client = &http.Client{Timeout: 20 * time.Millisecond}
	response := protectedRequest(Protect(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { t.Fatal("handler ran") }), validator, map[string]struct{}{testAdminID: {}}), "Bearer token")
	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("status=%d", response.Code)
	}
}

func protectedRequest(handler http.Handler, authorization string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, "/api/admin/booking-requests", nil)
	if authorization != "" {
		req.Header.Set("Authorization", authorization)
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, req)
	return response
}
