package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/rovinocampoo/mmsu-tennis-club/apps/api/internal/config"
	"github.com/rovinocampoo/mmsu-tennis-club/apps/api/internal/training"
)

type scheduleStub struct{}

func (scheduleStub) ListUpcoming(context.Context, time.Time) ([]training.Session, error) {
	return []training.Session{}, nil
}

func (scheduleStub) ListActivePrograms(context.Context) ([]training.Program, error) {
	return []training.Program{}, nil
}

func TestPublicHealthAndScheduleRoutesRemainAvailable(t *testing.T) {
	handler := newAPIHandler(config.Config{}, training.NewService(scheduleStub{}), nil, nil, log.New(io.Discard, "", 0))
	for _, path := range []string{"/health", "/api/training-programs", "/api/training-sessions"} {
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, httptest.NewRequest(http.MethodGet, path, nil))
		if w.Code != http.StatusOK {
			t.Errorf("GET %s status=%d body=%s", path, w.Code, w.Body)
		}
	}
}

func TestPublicBookingEndpointsApplyIndependentRateLimits(t *testing.T) {
	handler := newAPIHandler(config.Config{}, training.NewService(nil), nil, nil, log.New(io.Discard, "", 0))

	for i := 0; i < bookingRequestLimit; i++ {
		request := httptest.NewRequest(http.MethodPost, "/api/booking-requests", strings.NewReader(`{}`))
		request.RemoteAddr = "192.0.2.20:4567"
		request.Header.Set("X-Forwarded-For", "198.51.100."+fmt.Sprint(i+1))
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Code != http.StatusBadRequest {
			t.Fatalf("booking request %d status=%d, want validation response below limit", i+1, response.Code)
		}
	}
	exceeded := httptest.NewRequest(http.MethodPost, "/api/booking-requests", strings.NewReader(`{}`))
	exceeded.RemoteAddr = "192.0.2.20:4567"
	exceeded.Header.Set("X-Forwarded-For", "203.0.113.99")
	exceededResponse := httptest.NewRecorder()
	handler.ServeHTTP(exceededResponse, exceeded)
	if exceededResponse.Code != http.StatusTooManyRequests || exceededResponse.Header().Get("Retry-After") == "" {
		t.Fatalf("booking request above configured limit status=%d Retry-After=%q", exceededResponse.Code, exceededResponse.Header().Get("Retry-After"))
	}

	statusRequest := httptest.NewRequest(http.MethodPost, "/api/booking-status", strings.NewReader(`{"tracking_token":"invalid"}`))
	statusRequest.RemoteAddr = "192.0.2.20:4567"
	statusResponse := httptest.NewRecorder()
	handler.ServeHTTP(statusResponse, statusRequest)
	if statusResponse.Code != http.StatusNotFound {
		t.Fatalf("status lookup was affected by booking-request limit: status=%d", statusResponse.Code)
	}
	for i := 1; i < bookingStatusLimit; i++ {
		request := httptest.NewRequest(http.MethodPost, "/api/booking-status", strings.NewReader(`{"tracking_token":"invalid"}`))
		request.RemoteAddr = "192.0.2.20:4567"
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Code != http.StatusNotFound {
			t.Fatalf("status lookup %d status=%d, want invalid-token response below limit", i+1, response.Code)
		}
	}
	statusExceeded := httptest.NewRequest(http.MethodPost, "/api/booking-status", strings.NewReader(`{"tracking_token":"invalid"}`))
	statusExceeded.RemoteAddr = "192.0.2.20:4567"
	statusExceededResponse := httptest.NewRecorder()
	handler.ServeHTTP(statusExceededResponse, statusExceeded)
	if statusExceededResponse.Code != http.StatusTooManyRequests || statusExceededResponse.Header().Get("Retry-After") == "" {
		t.Fatalf("status lookup above configured limit status=%d Retry-After=%q", statusExceededResponse.Code, statusExceededResponse.Header().Get("Retry-After"))
	}
}

func TestAllAdminRoutesRejectMissingAuthorization(t *testing.T) {
	handler := newAPIHandler(config.Config{}, training.NewService(nil), nil, nil, log.New(io.Discard, "", 0))
	for _, request := range []struct{ method, path, body string }{
		{http.MethodGet, "/api/admin/booking-requests", ""},
		{http.MethodGet, "/api/admin/players?q=Alex", ""},
		{http.MethodPost, "/api/admin/booking-requests/11111111-1111-4111-8111-111111111111/approve", `{"create_new":true}`},
		{http.MethodPost, "/api/admin/booking-requests/11111111-1111-4111-8111-111111111111/reject", `{}`},
		{http.MethodPost, "/api/admin/booking-requests/11111111-1111-4111-8111-111111111111/notification/retry", ""},
		{http.MethodGet, "/api/coach/availability", ""},
		{http.MethodGet, "/api/coach/sessions", ""},
		{http.MethodPost, "/api/coach/availability", `{"notes":"test"}`},
		{http.MethodDelete, "/api/coach/availability/11111111-1111-4111-8111-111111111111", ""},
		{http.MethodGet, "/api/admin/coach-availability", ""},
		{http.MethodGet, "/api/admin/training-programs", ""},
		{http.MethodGet, "/api/admin/training-sessions", ""},
		{http.MethodPost, "/api/admin/training-sessions", `{}`},
		{http.MethodPost, "/api/admin/training-sessions/batch", `{"sessions":[]}`},
		{http.MethodPut, "/api/admin/training-sessions/11111111-1111-4111-8111-111111111111", `{}`},
		{http.MethodPatch, "/api/admin/training-sessions/11111111-1111-4111-8111-111111111111", `{}`},
		{http.MethodPatch, "/api/admin/training-sessions/11111111-1111-4111-8111-111111111111/coach", `{"coach_id":null}`},
	} {
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, httptest.NewRequest(request.method, request.path, strings.NewReader(request.body)))
		if w.Code != http.StatusUnauthorized {
			t.Errorf("%s %s status=%d body=%s", request.method, request.path, w.Code, w.Body)
		}
	}
}

func TestCoachRoutesRequireAllowlistedUser(t *testing.T) {
	const coachID = "11111111-1111-4111-8111-111111111111"
	const otherID = "22222222-2222-4222-8222-222222222222"
	authServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		userID := otherID
		if r.Header.Get("Authorization") == "Bearer coach-token" {
			userID = coachID
		}
		_, _ = fmt.Fprintf(w, `{"id":%q}`, userID)
	}))
	defer authServer.Close()

	cfg := config.Config{
		SupabaseURL:            authServer.URL,
		SupabasePublishableKey: "public-key",
		CoachUserIDs:           map[string]struct{}{coachID: {}},
	}
	handler := newAPIHandler(cfg, training.NewService(nil), nil, nil, log.New(io.Discard, "", 0))
	for _, tc := range []struct {
		name, token string
		wantStatus  int
	}{
		{name: "allowlisted coach reaches protected handler", token: "coach-token", wantStatus: http.StatusServiceUnavailable},
		{name: "other authenticated user is forbidden", token: "other-token", wantStatus: http.StatusForbidden},
		{name: "missing token is unauthorized", wantStatus: http.StatusUnauthorized},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, path := range []string{"/api/coach/availability", "/api/coach/sessions"} {
				request := httptest.NewRequest(http.MethodGet, path, nil)
				if tc.token != "" {
					request.Header.Set("Authorization", "Bearer "+tc.token)
				}
				response := httptest.NewRecorder()
				handler.ServeHTTP(response, request)
				if response.Code != tc.wantStatus {
					t.Errorf("GET %s status=%d want=%d body=%s", path, response.Code, tc.wantStatus, response.Body)
				}
			}
		})
	}
}
