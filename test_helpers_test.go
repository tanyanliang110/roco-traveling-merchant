package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func testResult() CrawlResult {
	return CrawlResult{
		TimeSlots:   []ShopSlot{{Label: "08:00-12:00"}},
		Products:    []Product{{Name: "测试商品", IsOnSale: true}},
		OnSaleCount: 1,
		TotalCount:  1,
		UpdatedAt:   "2026-09-09 13:00:00",
	}
}

func testState() NotificationState {
	return NotificationState{
		Date: "2026-09-09",
		Sent: map[string]bool{"2026-09-09 | 08:00-12:00 | 测试商品": true},
	}
}

func testConfig() Config {
	return Config{
		Server: ServerConfig{Port: ":8008"},
		Crawl:  CrawlConfig{TargetURL: "https://example.test/merchant", Interval: 3},
	}
}

func beijingDate(year int, month time.Month, day, hour, minute, second int) time.Time {
	return time.Date(year, month, day, hour, minute, second, 0, time.FixedZone("CST", 8*3600))
}

func writeStateFixture(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "state.json")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}
