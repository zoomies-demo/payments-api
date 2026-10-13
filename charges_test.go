package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func fixedClock() func() time.Time {
	t := time.Date(2026, 10, 13, 9, 0, 0, 0, time.UTC)
	return func() time.Time { return t }
}

func post(t *testing.T, h http.Handler, key, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/v1/charges", strings.NewReader(body))
	if key != "" {
		req.Header.Set("Idempotency-Key", key)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestValidationNamesWhatIsWrong(t *testing.T) {
	for _, tc := range []struct {
		name string
		req  ChargeRequest
		want string
	}{
		{"zero amount", ChargeRequest{0, "GBP", "order-1"}, "positive whole number"},
		{"over the limit", ChargeRequest{100_000_01, "GBP", "order-1"}, "limit"},
		{"unknown currency", ChargeRequest{500, "JPY", "order-1"}, "not one of"},
		{"no reference", ChargeRequest{500, "GBP", "  "}, "reference is required"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.req.Validate()
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("Validate() = %v; want an error saying %q", err, tc.want)
			}
		})
	}
}

// A client that times out and retries must not take the money twice.
func TestTheSameIdempotencyKeyReturnsTheFirstCharge(t *testing.T) {
	h := newHandler(NewLedger(fixedClock()))
	body := `{"amount":1250,"currency":"gbp","reference":"order-7"}`

	first := post(t, h, "key-1", body)
	if first.Code != http.StatusCreated {
		t.Fatalf("first charge answered %d; want 201", first.Code)
	}
	second := post(t, h, "key-1", body)
	if second.Code != http.StatusOK || second.Header().Get("Idempotent-Replay") != "true" {
		t.Fatalf("retry answered %d, replay=%q; want 200 and a replay header",
			second.Code, second.Header().Get("Idempotent-Replay"))
	}
	if first.Body.String() != second.Body.String() {
		t.Fatalf("the retry returned a different charge:\n%s\n%s", first.Body, second.Body)
	}
}

func TestAKeyReusedForADifferentChargeIsRefused(t *testing.T) {
	h := newHandler(NewLedger(fixedClock()))
	post(t, h, "key-1", `{"amount":1250,"currency":"GBP","reference":"order-7"}`)
	rec := post(t, h, "key-1", `{"amount":9999,"currency":"GBP","reference":"order-7"}`)
	if rec.Code != http.StatusConflict {
		t.Fatalf("answered %d; want 409", rec.Code)
	}
}

func TestACreatedChargeCanBeReadBack(t *testing.T) {
	h := newHandler(NewLedger(fixedClock()))
	post(t, h, "", `{"amount":300,"currency":"EUR","reference":"order-9"}`)

	req := httptest.NewRequest(http.MethodGet, "/v1/charges/ch_000001", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"currency":"EUR"`) {
		t.Fatalf("GET answered %d: %s", rec.Code, rec.Body)
	}

	missing := httptest.NewRecorder()
	h.ServeHTTP(missing, httptest.NewRequest(http.MethodGet, "/v1/charges/ch_999999", nil))
	if missing.Code != http.StatusNotFound {
		t.Fatalf("a missing charge answered %d; want 404", missing.Code)
	}
}

func TestAnUnknownFieldIsRefusedRatherThanIgnored(t *testing.T) {
	h := newHandler(NewLedger(fixedClock()))
	rec := post(t, h, "", `{"amount":300,"currency":"EUR","reference":"r","amout":1}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("a misspelt field answered %d; want 400", rec.Code)
	}
}
