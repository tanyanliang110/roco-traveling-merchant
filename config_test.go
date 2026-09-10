package main

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestApplyEnvironmentOverridesSendKey(t *testing.T) {
	cfg := Config{ServerChan: ServerChanConfig{SendKey: "file-key"}}

	got := ApplyEnvironment(cfg, func(name string) string {
		if name == "SERVERCHAN_SENDKEY" {
			return "env-value"
		}
		return ""
	})

	if got.ServerChan.SendKey != "env-value" {
		t.Fatalf("got %q", got.ServerChan.SendKey)
	}
}

func TestApplyEnvironmentKeepsFileValueWhenEnvironmentIsEmpty(t *testing.T) {
	cfg := Config{ServerChan: ServerChanConfig{SendKey: "file-key"}}

	got := ApplyEnvironment(cfg, func(string) string { return "" })

	if got.ServerChan.SendKey != "file-key" {
		t.Fatalf("got %q", got.ServerChan.SendKey)
	}
}

func TestLoadConfigReturnsDefaultsWhenFileDoesNotExist(t *testing.T) {
	got, err := LoadConfig(filepath.Join(t.TempDir(), "missing.json"))
	if err != nil {
		t.Fatal(err)
	}

	want := Config{
		Server: ServerConfig{Port: ":8008"},
		Crawl: CrawlConfig{
			TargetURL: "https://www.onebiji.com/hykb_tools/comm/lkwgmerchant/preview.php?id=1&immgj=0",
			Interval:  3,
		},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v, want %#v", got, want)
	}
}

func TestLoadConfigRejectsMalformedJSON(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte(`{"server":`), 0o600); err != nil {
		t.Fatal(err)
	}

	if _, err := LoadConfig(path); err == nil {
		t.Fatal("expected malformed JSON error")
	}
}
