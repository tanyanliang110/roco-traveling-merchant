package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestResolveServerChanEndpoint(t *testing.T) {
	tests := []struct {
		key  string
		want string
	}{
		{"SCTabc", "https://sctapi.ftqq.com/SCTabc.send"},
		{"sctp123tABC", "https://123.push.ft07.com/send/sctp123tABC.send"},
		{"https://push.example.test/hook", "https://push.example.test/hook"},
	}
	for _, tt := range tests {
		t.Run(tt.key, func(t *testing.T) {
			got, err := ResolveServerChanEndpoint(tt.key)
			if err != nil {
				t.Fatal(err)
			}
			if got.String() != tt.want {
				t.Fatalf("%q != %q", got, tt.want)
			}
		})
	}

	if _, err := ResolveServerChanEndpoint("bad-key"); err == nil {
		t.Fatal("expected invalid-key error")
	}
}

func TestResolveServerChanEndpointRejectsMalformedKeys(t *testing.T) {
	tests := []string{
		"SCT abc",
		"SCTabc/def",
		"SCTabc?title=value",
		"SCTabc#fragment",
		"SCTabc:extra",
		"sctp123tABC DEF",
		"sctp123tABC/DEF",
		"sctp123tABC?title=value",
		"sctp123tABC#fragment",
		"sctp123tABC:extra",
	}

	for _, key := range tests {
		t.Run(key, func(t *testing.T) {
			if _, err := ResolveServerChanEndpoint(key); err == nil {
				t.Fatalf("ResolveServerChanEndpoint(%q) succeeded, want error", key)
			}
		})
	}
}

func TestResolveServerChanEndpointRejectsCustomURLWithoutHostname(t *testing.T) {
	tests := []string{
		"https://:443/hook",
		"http://:80/hook",
		"https://user@:443/hook",
	}

	for _, endpoint := range tests {
		t.Run(endpoint, func(t *testing.T) {
			if _, err := ResolveServerChanEndpoint(endpoint); err == nil {
				t.Fatalf("ResolveServerChanEndpoint(%q) succeeded, want error", endpoint)
			}
		})
	}
}

func TestServerChanClientRetriesTemporaryStatusAndPostsForm(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if r.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", r.Method)
		}
		if err := r.ParseForm(); err != nil {
			t.Errorf("parse form: %v", err)
		}
		want := url.Values{
			"title": {"merchant update"},
			"desp":  {"two items available"},
			"tags":  {"Roco-API"},
		}
		if r.Form.Encode() != want.Encode() {
			t.Errorf("form = %q, want %q", r.Form.Encode(), want.Encode())
		}
		w.Header().Set("Content-Type", "application/json")
		if requests == 1 {
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = fmt.Fprint(w, `{"message":"try later"}`)
			return
		}
		_, _ = fmt.Fprint(w, `{"code":0,"message":"ok"}`)
	}))
	t.Cleanup(server.Close)

	client := ServerChanClient{HTTPClient: server.Client(), MaxAttempts: 2}
	if err := client.Send(context.Background(), server.URL, "merchant update", "two items available"); err != nil {
		t.Fatal(err)
	}
	if requests != 2 {
		t.Fatalf("requests = %d, want 2", requests)
	}
}

func TestServerChanClientDoesNotRetryDeterministicFailureOrLeakEndpoint(t *testing.T) {
	requests := 0
	var endpoint string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests++
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"code":    http.StatusUnauthorized,
			"message": "rejected " + endpoint,
		})
	}))
	t.Cleanup(server.Close)
	endpoint = server.URL

	client := ServerChanClient{HTTPClient: server.Client(), MaxAttempts: 3}
	err := client.Send(context.Background(), server.URL, "title", "body")
	if err == nil {
		t.Fatal("expected unauthorized error")
	}
	if requests != 1 {
		t.Fatalf("requests = %d, want 1", requests)
	}
	if strings.Contains(err.Error(), server.URL) {
		t.Fatalf("error leaked endpoint: %v", err)
	}
}

func TestServerChanClientDoesNotRetryStatusAbove599(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests++
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(600)
		_, _ = fmt.Fprint(w, `{"message":"invalid status"}`)
	}))
	t.Cleanup(server.Close)

	client := ServerChanClient{HTTPClient: server.Client(), MaxAttempts: 3}
	if err := client.Send(context.Background(), server.URL, "title", "body"); err == nil {
		t.Fatal("expected status error")
	}
	if requests != 1 {
		t.Fatalf("requests = %d, want 1", requests)
	}
}

func TestServerChanClientRequiresExplicitSuccessCode(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"message":"ambiguous"}`)
	}))
	t.Cleanup(server.Close)

	client := ServerChanClient{HTTPClient: server.Client(), MaxAttempts: 1}
	if err := client.Send(context.Background(), server.URL, "title", "body"); err == nil {
		t.Fatal("expected missing success-code error")
	}
}

func TestServerChanClientAcceptsExplicitZeroErrno(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"errno":0,"message":"ok"}`)
	}))
	t.Cleanup(server.Close)

	client := ServerChanClient{HTTPClient: server.Client(), MaxAttempts: 1}
	if err := client.Send(context.Background(), server.URL, "title", "body"); err != nil {
		t.Fatal(err)
	}
}
