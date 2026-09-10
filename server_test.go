package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"testing"
	"time"
)

// TestRefreshKeepsLastGoodCacheOnFailure fails if a failed fetch replaces a
// previously successful result with an empty or partial cache.
func TestRefreshKeepsLastGoodCacheOnFailure(t *testing.T) {
	merchantHTML, err := os.ReadFile("testdata/merchant.html")
	if err != nil {
		t.Fatal(err)
	}
	srv := newTestServerWithSequence(t, merchantHTML, http.StatusBadGateway)

	if err := srv.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	before := srv.snapshot()

	if err := srv.Refresh(context.Background()); err == nil {
		t.Fatal("expected refresh error")
	}
	if got := srv.snapshot(); !reflect.DeepEqual(before, got) {
		t.Fatalf("cache changed after failed refresh: before = %#v, after = %#v", before, got)
	}
}

// TestHandlerKeepsExistingRoutes fails if the local API stops serving one of
// its established public paths.
func TestHandlerKeepsExistingRoutes(t *testing.T) {
	srv := newServerWithResult(testResult())
	for _, path := range []string{"/", "/api/products", "/api/onsale"} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		rec := httptest.NewRecorder()
		srv.Handler().ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("%s: status = %d, want %d", path, rec.Code, http.StatusOK)
		}
	}
}

func newTestServerWithSequence(t *testing.T, merchantHTML []byte, failureStatus int) *Server {
	t.Helper()
	requests := 0
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if requests == 1 {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write(merchantHTML)
			return
		}
		w.WriteHeader(failureStatus)
	}))
	t.Cleanup(upstream.Close)

	return &Server{
		client: upstream.Client(),
		config: Config{Crawl: CrawlConfig{TargetURL: upstream.URL, Interval: 3}},
		now: func() time.Time {
			return beijingDate(2026, time.September, 9, 13, 0, 0)
		},
	}
}

func newServerWithResult(result CrawlResult) *Server {
	return &Server{
		cache:  result,
		client: &http.Client{Timeout: time.Second},
		config: testConfig(),
		now:    time.Now,
	}
}
