package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"path/filepath"
	"strings"
	"time"
)

// OnceDependencies contains the external boundaries used by a single run.
type OnceDependencies struct {
	Fetch  func(context.Context) (CrawlResult, error)
	Notify func(context.Context, []Product) error
	Now    func() time.Time
}

// RunOnce fetches, optionally notifies, and transactionally publishes one
// complete static snapshot.
func RunOnce(ctx context.Context, cfg Config, outputDir, statePath string, deps OnceDependencies) error {
	if cfg.ServerChan.SendKey != "" {
		if _, err := ResolveServerChanEndpoint(cfg.ServerChan.SendKey); err != nil {
			return fmt.Errorf("validate ServerChan SendKey: %w", err)
		}
	}
	if ctx == nil {
		ctx = context.Background()
	}
	clock := deps.Now
	if clock == nil {
		clock = time.Now
	}
	now := clock()

	state, err := LoadNotificationState(statePath, now)
	if err != nil {
		return fmt.Errorf("load notification state: %w", err)
	}
	if deps.Fetch == nil {
		return errors.New("fetch dependency is required")
	}
	result, err := deps.Fetch(ctx)
	if err != nil {
		return fmt.Errorf("fetch products: %w", err)
	}
	if err := validateOnceResult(result); err != nil {
		return err
	}

	onSale := make([]Product, 0, result.OnSaleCount)
	for _, product := range result.Products {
		if product.IsOnSale {
			onSale = append(onSale, product)
		}
	}
	pending := state.Pending(onSale, now)
	if serverChanConfigured(cfg.ServerChan.SendKey) && len(pending) > 0 {
		if deps.Notify == nil {
			return errors.New("notify dependency is required")
		}
		if err := deps.Notify(ctx, pending); err != nil {
			return fmt.Errorf("notify pending products: %w", err)
		}
		state.MarkSent(pending, now)
	}

	if err := WriteStaticOutput(outputDir, result, state); err != nil {
		return fmt.Errorf("publish static output: %w", err)
	}
	return nil
}

func validateOnceResult(result CrawlResult) error {
	if len(result.Products) == 0 {
		return errors.New("fetched result contains no products")
	}
	for _, product := range result.Products {
		if strings.TrimSpace(product.Name) == "" {
			return errors.New("fetched result contains a product without a name")
		}
	}
	return nil
}

func serverChanConfigured(sendKey string) bool {
	return sendKey != ""
}

var runClock = time.Now

// run parses the command line and selects the existing local server or one-shot
// static publishing mode.
func run(args []string, getenv func(string) string) error {
	flags := flag.NewFlagSet("roco-api", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	onceMode := flags.Bool("once", false, "fetch and publish one static snapshot")
	outputArg := flags.String("output", "", "static output directory")
	stateArg := flags.String("state", "", "notification state file")
	configPath := flags.String("config", "config.json", "configuration file")
	if err := flags.Parse(args); err != nil {
		return fmt.Errorf("parse command line: %w", err)
	}
	if flags.NArg() != 0 {
		return fmt.Errorf("unexpected command-line arguments: %s", strings.Join(flags.Args(), " "))
	}

	var outputDir, statePath string
	if *onceMode {
		if strings.TrimSpace(*outputArg) == "" {
			return errors.New("usage: roco-api -once -output <directory> -state <directory>/state.json; -output is required")
		}
		if strings.TrimSpace(*stateArg) == "" {
			return errors.New("usage: roco-api -once -output <directory> -state <directory>/state.json; -state is required")
		}
		var err error
		outputDir, err = filepath.Abs(*outputArg)
		if err != nil {
			return fmt.Errorf("resolve -output: %w", err)
		}
		statePath, err = filepath.Abs(*stateArg)
		if err != nil {
			return fmt.Errorf("resolve -state: %w", err)
		}
		expectedStatePath := filepath.Join(outputDir, "state.json")
		if statePath != expectedStatePath {
			return fmt.Errorf("-state must resolve exactly to %s", expectedStatePath)
		}
	}

	cfg, err := LoadConfig(*configPath)
	if err != nil {
		return err
	}
	cfg = ApplyEnvironment(cfg, getenv)
	appConfig = cfg

	if !*onceMode {
		initPushTracker()
		return NewServer(cfg).Run(context.Background())
	}
	if !serverChanConfigured(cfg.ServerChan.SendKey) {
		log.Println("[通知] 推送未启用（未配置 SERVERCHAN_SENDKEY）")
	}

	merchantClient := &http.Client{Timeout: 15 * time.Second}
	notificationClient := ServerChanClient{
		HTTPClient:  &http.Client{Timeout: 10 * time.Second},
		MaxAttempts: 3,
		BaseDelay:   500 * time.Millisecond,
	}
	clock := runClock
	var fetchedAt time.Time
	deps := OnceDependencies{
		Now: func() time.Time {
			fetchedAt = clock()
			return fetchedAt
		},
		Fetch: func(ctx context.Context) (CrawlResult, error) {
			if fetchedAt.IsZero() {
				fetchedAt = clock()
			}
			return FetchProducts(ctx, merchantClient, cfg.Crawl.TargetURL, fetchedAt)
		},
		Notify: func(ctx context.Context, products []Product) error {
			title := fmt.Sprintf("🏪 远行商人更新 · %d 件在售", len(products))
			slot := "未知时段"
			if len(products) > 0 && products[0].SlotLabel != "" {
				slot = products[0].SlotLabel
			}
			return notificationClient.Send(ctx, cfg.ServerChan.SendKey, title, buildOncePushMessage(products, slot))
		},
	}
	return RunOnce(context.Background(), cfg, outputDir, statePath, deps)
}
