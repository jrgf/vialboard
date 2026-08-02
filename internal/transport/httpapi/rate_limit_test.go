package httpapi

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	vial "github.com/jrgf/go-vial"
)

func TestRequestLimiter(t *testing.T) {
	now := time.Date(2026, time.August, 2, 12, 0, 0, 0, time.UTC)
	limiter := newRequestLimiter(2, time.Minute)
	limiter.now = func() time.Time { return now }
	app := vial.New()
	app.Post("/login", limiter.middleware(func(context *vial.Context) error {
		return context.NoContent(http.StatusNoContent)
	}))

	request := func() *httptest.ResponseRecorder {
		response := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/login", nil)
		req.RemoteAddr = "192.0.2.1:1234"
		app.ServeHTTP(response, req)
		return response
	}

	if status := request().Code; status != http.StatusNoContent {
		t.Fatalf("first status = %d", status)
	}
	if status := request().Code; status != http.StatusNoContent {
		t.Fatalf("second status = %d", status)
	}
	limited := request()
	if limited.Code != http.StatusTooManyRequests || limited.Header().Get("Retry-After") != "60" {
		t.Fatalf("limited status = %d, retry = %q", limited.Code, limited.Header().Get("Retry-After"))
	}

	now = now.Add(time.Minute)
	if status := request().Code; status != http.StatusNoContent {
		t.Fatalf("reset status = %d", status)
	}
}
