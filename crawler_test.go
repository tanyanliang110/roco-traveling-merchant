package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
)

// This test fails if parsing stops deriving product availability from data-time.
func TestParseProductsClassifiesProductTimes(t *testing.T) {
	fixture, err := os.Open("testdata/merchant.html")
	if err != nil {
		t.Fatal(err)
	}
	defer fixture.Close()

	now := time.Date(2026, 9, 9, 5, 0, 0, 0, time.UTC) // 北京时间 13:00
	got, err := ParseProducts(fixture, now)
	if err != nil {
		t.Fatal(err)
	}
	if got.TotalCount != 3 || got.OnSaleCount != 1 {
		t.Fatalf("counts = %d/%d", got.OnSaleCount, got.TotalCount)
	}
	if got.Products[1].StartAt != 1788926400 || got.Products[1].EndAt != 1788940800 {
		t.Fatalf("active product boundaries = %d/%d, want 1788926400/1788940800", got.Products[1].StartAt, got.Products[1].EndAt)
	}
	if !got.Products[1].IsOnSale {
		t.Fatalf("active product = %#v", got.Products[1])
	}
	if !got.Products[0].HasEnded || got.Products[2].IsOnSale || !got.Products[2].IsUpcoming {
		t.Fatalf("states = %#v", got.Products)
	}
}

// This test fails if a UTC caller timezone leaks into the user-facing display
// timestamp instead of consistently presenting Beijing time.
func TestParseProductsDisplaysUpdatedAtInBeijingTime(t *testing.T) {
	fixture, err := os.Open("testdata/merchant.html")
	if err != nil {
		t.Fatal(err)
	}
	defer fixture.Close()

	now := time.Date(2026, time.September, 9, 0, 2, 0, 0, time.UTC)
	got, err := ParseProducts(fixture, now)
	if err != nil {
		t.Fatal(err)
	}
	if got.UpdatedAt != "2026-09-09 08:02:00" {
		t.Fatalf("updated_at = %q, want Beijing boundary time %q", got.UpdatedAt, "2026-09-09 08:02:00")
	}
}

// This test fails if EndAt remains inclusive or StartAt becomes exclusive.
// At the shared 16:00 boundary, the old product is ended and the next is live.
func TestParseProductsUsesHalfOpenSaleBoundaries(t *testing.T) {
	fixture, err := os.Open("testdata/merchant.html")
	if err != nil {
		t.Fatal(err)
	}
	defer fixture.Close()

	now := beijingDate(2026, time.September, 9, 16, 0, 0)
	got, err := ParseProducts(fixture, now)
	if err != nil {
		t.Fatal(err)
	}
	ending, starting := got.Products[1], got.Products[2]
	if ending.EndAt != now.Unix() || !ending.HasEnded || ending.IsOnSale || ending.IsUpcoming {
		t.Fatalf("product at EndAt = %#v, want ended only", ending)
	}
	if starting.StartAt != now.Unix() || starting.HasEnded || !starting.IsOnSale || starting.IsUpcoming {
		t.Fatalf("product at StartAt = %#v, want on sale only", starting)
	}
	if got.OnSaleCount != 1 {
		t.Fatalf("on_sale_count = %d, want 1 at shared boundary", got.OnSaleCount)
	}
}

// This test fails if an upstream markup change can be mistaken for an empty catalog.
func TestParseProductsRejectsEmptyResult(t *testing.T) {
	_, err := ParseProducts(strings.NewReader("<html></html>"), time.Now())
	if err == nil {
		t.Fatal("expected empty-result error")
	}
}

// This test fails if FetchProducts bypasses the supplied client or parser.
func TestFetchProductsParsesSuccessfulResponse(t *testing.T) {
	merchantHTML, err := os.ReadFile("testdata/merchant.html")
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/merchant" {
			t.Errorf("path = %q, want /merchant", r.URL.Path)
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write(merchantHTML)
	}))
	defer server.Close()

	got, err := FetchProducts(context.Background(), server.Client(), server.URL+"/merchant", beijingDate(2026, 9, 9, 13, 0, 0))
	if err != nil {
		t.Fatal(err)
	}
	if got.TotalCount != 3 || got.OnSaleCount != 1 {
		t.Fatalf("counts = %d/%d", got.OnSaleCount, got.TotalCount)
	}
}

// This test fails if FetchProducts accepts non-successful upstream HTTP responses.
func TestFetchProductsRejectsNonOKResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer server.Close()

	_, err := FetchProducts(context.Background(), server.Client(), server.URL, time.Now())
	if err == nil {
		t.Fatal("expected non-OK response error")
	}
}
