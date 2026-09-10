package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

var (
	serverChanKeyPattern  = regexp.MustCompile(`^SCT[A-Za-z0-9]+$`)
	serverChan3KeyPattern = regexp.MustCompile(`^sctp([0-9]+)t[A-Za-z0-9]+$`)
)

// ServerChanResponse is the response envelope returned by ServerChan APIs.
// Pointer fields distinguish an explicit zero from a missing field.
type ServerChanResponse struct {
	Code    *int   `json:"code"`
	Errno   *int   `json:"errno"`
	Message string `json:"message"`
}

// ResolveServerChanEndpoint resolves official, ServerChan³, and custom URLs.
func ResolveServerChanEndpoint(sendKey string) (*url.URL, error) {
	if strings.HasPrefix(sendKey, "http://") || strings.HasPrefix(sendKey, "https://") {
		endpoint, err := url.Parse(sendKey)
		if err != nil || endpoint.Hostname() == "" {
			return nil, fmt.Errorf("invalid ServerChan endpoint")
		}
		return endpoint, nil
	}

	if serverChanKeyPattern.MatchString(sendKey) {
		return &url.URL{
			Scheme: "https",
			Host:   "sctapi.ftqq.com",
			Path:   "/" + sendKey + ".send",
		}, nil
	}

	if matches := serverChan3KeyPattern.FindStringSubmatch(sendKey); matches != nil {
		return &url.URL{
			Scheme: "https",
			Host:   matches[1] + ".push.ft07.com",
			Path:   "/send/" + sendKey + ".send",
		}, nil
	}

	return nil, fmt.Errorf("invalid ServerChan send key")
}

// ServerChanClient sends form-encoded notifications with bounded retries.
type ServerChanClient struct {
	HTTPClient  *http.Client
	MaxAttempts int
	BaseDelay   time.Duration
}

// Send posts a notification. It retries only network errors, 429, and 5xx.
func (c ServerChanClient) Send(ctx context.Context, sendKey, title, body string) error {
	endpoint, err := ResolveServerChanEndpoint(sendKey)
	if err != nil {
		return err
	}
	if ctx == nil {
		ctx = context.Background()
	}
	client := c.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	attempts := c.MaxAttempts
	if attempts <= 0 {
		attempts = 3
	}

	var lastErr error
	for attempt := 1; attempt <= attempts; attempt++ {
		retry, sendErr := sendServerChanAttempt(ctx, client, endpoint, sendKey, title, body)
		if sendErr == nil {
			return nil
		}
		lastErr = sendErr
		if !retry || attempt == attempts {
			return sendErr
		}
		if err := waitForServerChanRetry(ctx, c.BaseDelay); err != nil {
			return err
		}
	}
	return lastErr
}

func sendServerChanAttempt(ctx context.Context, client *http.Client, endpoint *url.URL, sendKey, title, body string) (bool, error) {
	form := url.Values{
		"title": {title},
		"desp":  {body},
		"tags":  {"Roco-API"},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint.String(), strings.NewReader(form.Encode()))
	if err != nil {
		return false, fmt.Errorf("failed to create ServerChan request")
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := client.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return false, ctx.Err()
		}
		return true, fmt.Errorf("ServerChan network request to %s failed", endpoint.Host)
	}
	defer resp.Body.Close()

	var result ServerChanResponse
	decodeErr := json.NewDecoder(resp.Body).Decode(&result)
	message := sanitizeServerChanMessage(result.Message, sendKey, endpoint.String())
	if resp.StatusCode == http.StatusTooManyRequests ||
		(resp.StatusCode >= http.StatusInternalServerError && resp.StatusCode <= 599) {
		return true, serverChanStatusError(endpoint.Host, resp.StatusCode, message)
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return false, serverChanStatusError(endpoint.Host, resp.StatusCode, message)
	}
	if decodeErr != nil {
		return false, fmt.Errorf("ServerChan response from %s was not valid JSON", endpoint.Host)
	}
	if (result.Code != nil && *result.Code == 0) || (result.Errno != nil && *result.Errno == 0) {
		return false, nil
	}
	if message == "" {
		message = "response did not contain an explicit success code"
	}
	return false, fmt.Errorf("ServerChan request to %s failed: %s", endpoint.Host, message)
}

func serverChanStatusError(host string, status int, message string) error {
	if message == "" {
		return fmt.Errorf("ServerChan request to %s returned status %d", host, status)
	}
	return fmt.Errorf("ServerChan request to %s returned status %d: %s", host, status, message)
}

func sanitizeServerChanMessage(message, sendKey, endpoint string) string {
	message = strings.ReplaceAll(message, endpoint, "[redacted]")
	message = strings.ReplaceAll(message, sendKey, "[redacted]")
	return message
}

func waitForServerChanRetry(ctx context.Context, delay time.Duration) error {
	if delay <= 0 {
		return nil
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// SendServerChan preserves the existing local notification call site.
func SendServerChan(title, body string) {
	if !ServerChanEnabled() {
		return
	}
	client := ServerChanClient{MaxAttempts: 3, BaseDelay: 500 * time.Millisecond}
	if err := client.Send(context.Background(), appConfig.ServerChan.SendKey, title, body); err != nil {
		log.Printf("[错误] Server酱 推送失败: %v", err)
		return
	}
	log.Println("[完成] Server酱 推送成功")
}
