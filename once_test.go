package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// This test fails if once-mode notifications expose a localhost URL that a
// remote ServerChan recipient cannot open, or if local-service notifications
// lose their useful local page link.
func TestOncePushMessageOmitsLocalhostWhileLocalMessageKeepsIt(t *testing.T) {
	previousConfig := appConfig
	appConfig.Server.Port = ":8008"
	t.Cleanup(func() { appConfig = previousConfig })
	products := []Product{{Name: "在售商品", Price: "100"}}

	onceMessage := buildOncePushMessage(products, "12:00-16:00")
	if strings.Contains(onceMessage, "localhost") {
		t.Fatalf("once notification contains unusable local URL: %q", onceMessage)
	}
	localMessage := buildPushMessage(products, "12:00-16:00")
	if !strings.Contains(localMessage, "http://localhost:8008") {
		t.Fatalf("local notification lost its local page URL: %q", localMessage)
	}
}

// This test fails if a successful once run notifies non-sale products, omits
// persisted delivery state, or sends the same product again on the next run.
func TestRunOnceNotifiesPendingOnSaleProductsAndPersistsState(t *testing.T) {
	dir := t.TempDir()
	statePath := filepath.Join(dir, "state.json")
	now := beijingDate(2026, time.September, 9, 13, 2, 0)
	cfg := testConfig()
	cfg.ServerChan.SendKey = "SCTtest"
	var notified [][]Product
	deps := OnceDependencies{
		Fetch: func(context.Context) (CrawlResult, error) { return pageTestResult(), nil },
		Notify: func(_ context.Context, products []Product) error {
			notified = append(notified, append([]Product(nil), products...))
			return nil
		},
		Now: func() time.Time { return now },
	}

	if err := RunOnce(context.Background(), cfg, dir, statePath, deps); err != nil {
		t.Fatal(err)
	}
	if err := RunOnce(context.Background(), cfg, dir, statePath, deps); err != nil {
		t.Fatal(err)
	}
	if len(notified) != 1 {
		t.Fatalf("notification calls = %d, want 1", len(notified))
	}
	if len(notified[0]) != 1 || notified[0][0].Name != "在售商品" {
		t.Fatalf("notified products = %#v, want only the on-sale product", notified[0])
	}
	state, err := LoadNotificationState(statePath, now)
	if err != nil {
		t.Fatal(err)
	}
	wantKey := "2026-09-09 | 12:00-16:00 | 在售商品"
	if len(state.Sent) != 1 || !state.Sent[wantKey] {
		t.Fatalf("persisted state = %#v, want only %q", state, wantKey)
	}
}

// This test fails if RunOnce publishes partial data or calls notifications
// after the merchant fetch has failed.
func TestRunOnceFetchFailureDoesNotNotifyOrChangeOldFiles(t *testing.T) {
	dir, before := writeOldStaticOutput(t)
	fetchErr := errors.New("merchant unavailable")
	notifyCalls := 0
	cfg := testConfig()
	cfg.ServerChan.SendKey = "SCTtest"

	err := RunOnce(context.Background(), cfg, dir, filepath.Join(dir, "state.json"), OnceDependencies{
		Fetch:  func(context.Context) (CrawlResult, error) { return CrawlResult{}, fetchErr },
		Notify: func(context.Context, []Product) error { notifyCalls++; return nil },
		Now:    func() time.Time { return beijingDate(2026, time.September, 9, 13, 2, 0) },
	})
	if !errors.Is(err, fetchErr) {
		t.Fatalf("error = %v, want fetch error", err)
	}
	if notifyCalls != 0 {
		t.Fatalf("notification calls = %d, want 0", notifyCalls)
	}
	assertStaticOutputUnchanged(t, dir, before)
}

// This test fails if a failed notification is recorded as delivered or if a
// notification failure replaces any member of the old static snapshot.
func TestRunOnceNotifyFailureDoesNotMarkStateOrChangeOldFiles(t *testing.T) {
	dir, before := writeOldStaticOutput(t)
	notifyErr := errors.New("notification rejected")
	cfg := testConfig()
	cfg.ServerChan.SendKey = "SCTtest"

	err := RunOnce(context.Background(), cfg, dir, filepath.Join(dir, "state.json"), OnceDependencies{
		Fetch:  func(context.Context) (CrawlResult, error) { return pageTestResult(), nil },
		Notify: func(context.Context, []Product) error { return notifyErr },
		Now:    func() time.Time { return beijingDate(2026, time.September, 9, 13, 2, 0) },
	})
	if !errors.Is(err, notifyErr) {
		t.Fatalf("error = %v, want notification error", err)
	}
	assertStaticOutputUnchanged(t, dir, before)
}

// This test fails if once mode treats an absent SendKey as an error, invokes a
// notifier, or suppresses a future notification by marking products sent.
func TestRunOnceWithoutSendKeyPublishesButDoesNotMarkProductsSent(t *testing.T) {
	dir := t.TempDir()
	statePath := filepath.Join(dir, "state.json")
	now := beijingDate(2026, time.September, 9, 13, 2, 0)
	cfg := testConfig()
	deps := OnceDependencies{
		Fetch: func(context.Context) (CrawlResult, error) { return pageTestResult(), nil },
		Notify: func(context.Context, []Product) error {
			t.Fatal("Notify called without a SendKey")
			return nil
		},
		Now: func() time.Time { return now },
	}

	if err := RunOnce(context.Background(), cfg, dir, statePath, deps); err != nil {
		t.Fatal(err)
	}
	for _, name := range staticFileNames {
		if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
			t.Fatalf("%s was not published: %v", name, err)
		}
	}
	state, err := LoadNotificationState(statePath, now)
	if err != nil {
		t.Fatal(err)
	}
	if len(state.Sent) != 0 {
		t.Fatalf("state was marked without delivery: %#v", state)
	}

	cfg.ServerChan.SendKey = "SCTtest"
	notifyCalls := 0
	deps.Notify = func(context.Context, []Product) error { notifyCalls++; return nil }
	if err := RunOnce(context.Background(), cfg, dir, statePath, deps); err != nil {
		t.Fatal(err)
	}
	if notifyCalls != 1 {
		t.Fatalf("notification calls after configuring SendKey = %d, want 1", notifyCalls)
	}
}

// This test fails if an invalid nonempty SendKey is only validated while
// sending. The persisted key makes the fetched product non-pending, but bad
// configuration must still fail before fetch and leave the snapshot intact.
func TestRunOnceRejectsInvalidSendKeyBeforeFetchWhenNothingIsPending(t *testing.T) {
	dir := t.TempDir()
	now := beijingDate(2026, time.September, 9, 13, 2, 0)
	state := NotificationState{
		Date: "2026-09-09",
		Sent: map[string]bool{
			"2026-09-09 | 12:00-16:00 | 在售商品": true,
		},
	}
	if err := WriteStaticOutput(dir, pageTestResult(), state); err != nil {
		t.Fatal(err)
	}
	before := readStaticOutput(t, dir)
	cfg := testConfig()
	cfg.ServerChan.SendKey = "invalid-send-key"
	fetchCalls := 0

	err := RunOnce(context.Background(), cfg, dir, filepath.Join(dir, "state.json"), OnceDependencies{
		Fetch: func(context.Context) (CrawlResult, error) {
			fetchCalls++
			return pageTestResult(), nil
		},
		Notify: func(context.Context, []Product) error {
			t.Fatal("Notify called for invalid configuration")
			return nil
		},
		Now: func() time.Time { return now },
	})
	if err == nil || !strings.Contains(err.Error(), "invalid ServerChan send key") {
		t.Fatalf("error = %v, want invalid SendKey error", err)
	}
	if fetchCalls != 0 {
		t.Fatalf("fetch calls = %d, want 0 before invalid configuration is rejected", fetchCalls)
	}
	assertStaticOutputUnchanged(t, dir, before)
}

// This test fails if a successful CLI once run silently skips notifications
// or reveals configuration values while explaining why it skipped them.
func TestRunOnceCLIWithoutSendKeyLogsPushDisabled(t *testing.T) {
	merchantHTML, err := os.ReadFile("testdata/merchant.html")
	if err != nil {
		t.Fatal(err)
	}
	merchant := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(merchantHTML)
	}))
	t.Cleanup(merchant.Close)

	const privateUID = "private-user-id"
	configPath := writeOnceConfig(t, Config{
		Crawl:      CrawlConfig{TargetURL: merchant.URL},
		ServerChan: ServerChanConfig{UID: privateUID},
	})
	outputDir := filepath.Join(t.TempDir(), "public")

	var logs bytes.Buffer
	oldOutput := log.Writer()
	oldFlags := log.Flags()
	oldPrefix := log.Prefix()
	log.SetOutput(&logs)
	log.SetFlags(0)
	log.SetPrefix("")
	t.Cleanup(func() {
		log.SetOutput(oldOutput)
		log.SetFlags(oldFlags)
		log.SetPrefix(oldPrefix)
	})

	err = run([]string{
		"-once",
		"-config", configPath,
		"-output", outputDir,
		"-state", filepath.Join(outputDir, "state.json"),
	}, func(string) string { return "" })
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(logs.String(), "推送未启用") {
		t.Fatalf("logs = %q, want an explicit push-disabled message", logs.String())
	}
	if strings.Contains(logs.String(), privateUID) || strings.Contains(logs.String(), merchant.URL) {
		t.Fatalf("logs exposed configuration: %q", logs.String())
	}
}

// This test fails if an injected fetch dependency can bypass the empty-result
// guard and replace a valid static snapshot with an empty catalog.
func TestRunOnceRejectsZeroProductDependencyResult(t *testing.T) {
	dir, before := writeOldStaticOutput(t)
	notifyCalls := 0
	cfg := testConfig()
	cfg.ServerChan.SendKey = "SCTtest"

	err := RunOnce(context.Background(), cfg, dir, filepath.Join(dir, "state.json"), OnceDependencies{
		Fetch:  func(context.Context) (CrawlResult, error) { return CrawlResult{}, nil },
		Notify: func(context.Context, []Product) error { notifyCalls++; return nil },
		Now:    func() time.Time { return beijingDate(2026, time.September, 9, 13, 2, 0) },
	})
	if err == nil {
		t.Fatal("expected zero-product result error")
	}
	if notifyCalls != 0 {
		t.Fatalf("notification calls = %d, want 0", notifyCalls)
	}
	assertStaticOutputUnchanged(t, dir, before)
}

// This test fails if validation permits unnamed products into notifications or
// public output.
func TestRunOnceRejectsProductWithoutName(t *testing.T) {
	dir := t.TempDir()
	result := CrawlResult{
		Products:    []Product{{IsOnSale: true, SlotLabel: "12:00-16:00"}},
		OnSaleCount: 1,
		TotalCount:  1,
	}
	err := RunOnce(context.Background(), Config{}, dir, filepath.Join(dir, "state.json"), OnceDependencies{
		Fetch:  func(context.Context) (CrawlResult, error) { return result, nil },
		Notify: func(context.Context, []Product) error { t.Fatal("Notify called"); return nil },
		Now:    func() time.Time { return beijingDate(2026, time.September, 9, 13, 2, 0) },
	})
	if err == nil {
		t.Fatal("expected unnamed-product validation error")
	}
	if _, statErr := os.Stat(filepath.Join(dir, "products.json")); !os.IsNotExist(statErr) {
		t.Fatalf("products.json exists after validation failure: %v", statErr)
	}
}

// This test fails if the orchestration fetches before validating the persisted
// notification state, which could cause duplicate sends after state damage.
func TestRunOnceLoadsStateBeforeFetching(t *testing.T) {
	dir := t.TempDir()
	statePath := filepath.Join(dir, "state.json")
	if err := os.WriteFile(statePath, []byte("{broken"), 0o600); err != nil {
		t.Fatal(err)
	}
	fetched := false
	err := RunOnce(context.Background(), testConfig(), dir, statePath, OnceDependencies{
		Fetch: func(context.Context) (CrawlResult, error) { fetched = true; return pageTestResult(), nil },
		Now:   func() time.Time { return beijingDate(2026, time.September, 9, 13, 2, 0) },
	})
	if err == nil {
		t.Fatal("expected corrupt state error")
	}
	if fetched {
		t.Fatal("merchant was fetched before persisted state was validated")
	}
}

func TestRunOnceCLIRequiresOutputAndExactStatePath(t *testing.T) {
	getenv := func(string) string { return "" }
	if err := run([]string{"-once"}, getenv); err == nil || !strings.Contains(err.Error(), "-output") {
		t.Fatalf("missing-output error = %v", err)
	}

	dir := t.TempDir()
	err := run([]string{
		"-once",
		"-output", filepath.Join(dir, "public"),
		"-state", filepath.Join(dir, "state.json"),
	}, getenv)
	if err == nil || !strings.Contains(err.Error(), "state.json") {
		t.Fatalf("mismatched-state error = %v", err)
	}
}

// This test fails if CLI path comparison uses raw arguments instead of the
// normalized absolute output/state locations.
func TestRunOnceCLINormalizesOutputAndStatePaths(t *testing.T) {
	merchantHTML, err := os.ReadFile("testdata/merchant.html")
	if err != nil {
		t.Fatal(err)
	}
	merchant := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(merchantHTML)
	}))
	t.Cleanup(merchant.Close)

	dir := t.TempDir()
	configPath := writeOnceConfig(t, Config{Crawl: CrawlConfig{TargetURL: merchant.URL}})
	setRunClock(t, func() time.Time { return beijingDate(2026, time.September, 9, 13, 2, 0) })
	outputArg := filepath.Join(dir, "unused", "..", "public")
	stateArg := filepath.Join(dir, "public", "nested", "..", "state.json")
	if err := run([]string{
		"-once",
		"-config", configPath,
		"-output", outputArg,
		"-state", stateArg,
	}, func(string) string { return "" }); err != nil {
		t.Fatal(err)
	}
	for _, name := range staticFileNames {
		if _, err := os.Stat(filepath.Join(dir, "public", name)); err != nil {
			t.Fatalf("normalized output lacks %s: %v", name, err)
		}
	}
}

// This test fails if omitting -once no longer selects the existing local
// server mode. The deliberately invalid listen address makes that mode return
// immediately, so the test cannot leave a server running.
func TestRunDefaultsToLocalServerMode(t *testing.T) {
	merchantHTML, err := os.ReadFile("testdata/merchant.html")
	if err != nil {
		t.Fatal(err)
	}
	var merchantRequests atomic.Int32
	merchant := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		merchantRequests.Add(1)
		_, _ = w.Write(merchantHTML)
	}))
	t.Cleanup(merchant.Close)
	configPath := writeOnceConfig(t, Config{
		Server: ServerConfig{Port: "invalid-listen-address"},
		Crawl:  CrawlConfig{TargetURL: merchant.URL, Interval: 3},
	})

	err = run([]string{"-config", configPath}, func(string) string { return "" })
	if err == nil {
		t.Fatal("expected invalid local listen address error")
	}
	if merchantRequests.Load() != 1 {
		t.Fatalf("merchant requests = %d, want 1 from local server refresh", merchantRequests.Load())
	}
}

// This offline integration test fails if production run bypasses the real HTTP
// fetch/notifier boundaries, omits any output, or loses persisted deduplication
// between independent CLI invocations.
func TestRunOnceCLIIntegrationFetchesPublishesNotifiesAndDeduplicates(t *testing.T) {
	merchantHTML, err := os.ReadFile("testdata/merchant.html")
	if err != nil {
		t.Fatal(err)
	}
	var merchantRequests atomic.Int32
	merchant := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		merchantRequests.Add(1)
		if r.URL.Path != "/merchant" {
			t.Errorf("merchant path = %q, want /merchant", r.URL.Path)
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write(merchantHTML)
	}))
	t.Cleanup(merchant.Close)

	var notifyRequests atomic.Int32
	notifier := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		notifyRequests.Add(1)
		if r.URL.Path != "/send/test-secret" {
			t.Errorf("notification path = %q", r.URL.Path)
		}
		if err := r.ParseForm(); err != nil {
			t.Errorf("parse notification form: %v", err)
		}
		if r.Form.Get("title") == "" || !strings.Contains(r.Form.Get("desp"), "当前在售商品") {
			t.Errorf("notification form = %#v", r.Form)
		}
		if strings.Contains(r.Form.Get("desp"), "localhost") {
			t.Errorf("once notification contains unusable local URL: %q", r.Form.Get("desp"))
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"code":0,"message":"ok"}`)
	}))
	t.Cleanup(notifier.Close)

	dir := t.TempDir()
	outputDir := filepath.Join(dir, "public")
	statePath := filepath.Join(outputDir, "state.json")
	configPath := writeOnceConfig(t, Config{
		Crawl: CrawlConfig{TargetURL: merchant.URL + "/merchant", Interval: 3},
	})
	setRunClock(t, func() time.Time { return beijingDate(2026, time.September, 9, 13, 2, 0) })
	getenv := func(name string) string {
		if name == "SERVERCHAN_SENDKEY" {
			return notifier.URL + "/send/test-secret"
		}
		return ""
	}
	args := []string{"-once", "-config", configPath, "-output", outputDir, "-state", statePath}

	if err := run(args, getenv); err != nil {
		t.Fatal(err)
	}
	if err := run(args, getenv); err != nil {
		t.Fatal(err)
	}
	if merchantRequests.Load() != 2 {
		t.Fatalf("merchant requests = %d, want 2", merchantRequests.Load())
	}
	if notifyRequests.Load() != 1 {
		t.Fatalf("notification requests = %d, want 1", notifyRequests.Load())
	}
	for _, name := range staticFileNames {
		if _, err := os.Stat(filepath.Join(outputDir, name)); err != nil {
			t.Fatalf("missing %s: %v", name, err)
		}
	}
	state, err := LoadNotificationState(statePath, beijingDate(2026, time.September, 9, 13, 2, 0))
	if err != nil {
		t.Fatal(err)
	}
	if len(state.Sent) != 1 || !state.Sent["2026-09-09 | 12:00-16:00 | 当前在售商品"] {
		t.Fatalf("integration state = %#v", state)
	}
	for _, name := range staticFileNames {
		content, err := os.ReadFile(filepath.Join(outputDir, name))
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(content), "test-secret") || strings.Contains(string(content), notifier.URL) {
			t.Fatalf("%s contains notification secret or endpoint", name)
		}
	}
}

func writeOldStaticOutput(t *testing.T) (string, map[string][]byte) {
	t.Helper()
	dir := t.TempDir()
	if err := WriteStaticOutput(dir, testResult(), testState()); err != nil {
		t.Fatal(err)
	}
	return dir, readStaticOutput(t, dir)
}

func readStaticOutput(t *testing.T, dir string) map[string][]byte {
	t.Helper()
	files := make(map[string][]byte, len(staticFileNames))
	for _, name := range staticFileNames {
		content, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			t.Fatal(err)
		}
		files[name] = content
	}
	return files
}

func assertStaticOutputUnchanged(t *testing.T, dir string, before map[string][]byte) {
	t.Helper()
	after := readStaticOutput(t, dir)
	if !reflect.DeepEqual(after, before) {
		t.Fatalf("static output changed\nbefore: %#v\nafter:  %#v", before, after)
	}
}

func writeOnceConfig(t *testing.T, cfg Config) string {
	t.Helper()
	data, err := json.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func setRunClock(t *testing.T, clock func() time.Time) {
	t.Helper()
	previous := runClock
	runClock = clock
	t.Cleanup(func() { runClock = previous })
}
