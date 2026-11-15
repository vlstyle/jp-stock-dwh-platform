package jquants

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func newTestClient(t *testing.T, h http.HandlerFunc) *Client {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return New("test-key", 6000, WithBaseURL(srv.URL), WithRetryWait(time.Millisecond))
}

func TestDailyBarsPaginationAndHeader(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("x-api-key"); got != "test-key" {
			t.Errorf("x-api-key = %q", got)
		}
		if r.URL.Path != "/equities/bars/daily" || r.URL.Query().Get("date") != "2024-01-04" {
			t.Errorf("unexpected request: %s", r.URL)
		}
		switch r.URL.Query().Get("pagination_key") {
		case "":
			w.Write([]byte(`{"data":[{"Date":"2024-01-04","Code":"86970","C":2045.0}],"pagination_key":"k1"}`))
		case "k1":
			w.Write([]byte(`{"data":[{"Date":"2024-01-04","Code":"72030","C":null}]}`))
		default:
			t.Errorf("unexpected pagination_key")
		}
	})

	bars, err := c.DailyBars(context.Background(), "2024-01-04")
	if err != nil {
		t.Fatal(err)
	}
	if len(bars) != 2 {
		t.Fatalf("len = %d", len(bars))
	}
	if bars[0].C == nil || *bars[0].C != 2045.0 {
		t.Errorf("bars[0].C = %v", bars[0].C)
	}
	if bars[1].C != nil {
		t.Errorf("bars[1].C = %v, want nil", bars[1].C)
	}
}

func TestRetryOn429(t *testing.T) {
	var calls atomic.Int32
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) < 3 {
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		w.Write([]byte(`{"data":[{"Code":"86970"}]}`))
	})

	got, err := c.ListedInfo(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || calls.Load() != 3 {
		t.Errorf("len = %d, calls = %d", len(got), calls.Load())
	}
}

func TestAPIError(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		w.Write([]byte(`{"message":"out of range"}`))
	})

	_, err := c.DailyBars(context.Background(), "2024-01-04")
	apiErr, ok := err.(*APIError)
	if !ok || apiErr.Status != http.StatusForbidden {
		t.Fatalf("err = %v", err)
	}
}
