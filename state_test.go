package main

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestNotificationKeySeparatesSameNameAcrossSlotsAndDates(t *testing.T) {
	product := Product{Name: "紫莲刚玉", SlotLabel: "08:00-12:00"}
	sameNameOtherSlot := Product{Name: "紫莲刚玉", SlotLabel: "12:00-16:00"}
	now := beijingDate(2026, 9, 9, 12, 5, 0)
	nextDay := beijingDate(2026, 9, 10, 12, 5, 0)

	key := NotificationKey(product, now)
	if key != "2026-09-09 | 08:00-12:00 | 紫莲刚玉" {
		t.Fatalf("key = %q", key)
	}
	if key == NotificationKey(sameNameOtherSlot, now) {
		t.Fatal("same name in different slots must have different keys")
	}
	if key == NotificationKey(product, nextDay) {
		t.Fatal("same product on different dates must have different keys")
	}
}

func TestLoadNotificationStateMissingFileReturnsEmptyCurrentDay(t *testing.T) {
	path := t.TempDir() + "/state.json"
	state, err := LoadNotificationState(path, beijingDate(2026, 9, 9, 8, 2, 0))
	if err != nil {
		t.Fatal(err)
	}
	if state.Date != "2026-09-09" || len(state.Sent) != 0 {
		t.Fatalf("state = %#v", state)
	}
	if state.Sent == nil {
		t.Fatal("Sent must be initialized")
	}
}

func TestLoadNotificationStateRejectsCorruptJSON(t *testing.T) {
	path := writeStateFixture(t, `{not json`)
	if _, err := LoadNotificationState(path, beijingDate(2026, 9, 9, 8, 2, 0)); err == nil {
		t.Fatal("expected corrupt JSON error")
	}
}

func TestLoadNotificationStateResetsPreviousDay(t *testing.T) {
	path := writeStateFixture(t, `{"date":"2026-09-08","sent":{"old":true}}`)
	state, err := LoadNotificationState(path, beijingDate(2026, 9, 9, 8, 2, 0))
	if err != nil {
		t.Fatal(err)
	}
	if len(state.Sent) != 0 || state.Date != "2026-09-09" {
		t.Fatalf("state = %#v", state)
	}
}

func TestNotificationStateMarkSentLeavesNoPendingProducts(t *testing.T) {
	now := beijingDate(2026, 9, 9, 12, 5, 0)
	products := []Product{
		{Name: "紫莲刚玉", SlotLabel: "08:00-12:00"},
		{Name: "紫莲刚玉", SlotLabel: "12:00-16:00"},
	}
	state := NotificationState{Date: "2026-09-09", Sent: make(map[string]bool)}

	state.MarkSent(products, now)
	if pending := state.Pending(products, now); len(pending) != 0 {
		t.Fatalf("pending = %#v", pending)
	}
}

func TestSaveNotificationStateReplacesExistingState(t *testing.T) {
	path := writeStateFixture(t, `{"date":"2026-09-08","sent":{"old":true}}`)
	want := NotificationState{
		Date: "2026-09-09",
		Sent: map[string]bool{"2026-09-09 | 08:00-12:00 | 紫莲刚玉": true},
	}

	if err := SaveNotificationState(path, want); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) == `{"date":"2026-09-08","sent":{"old":true}}` {
		t.Fatalf("state file was not replaced: %s", data)
	}
	got, err := LoadNotificationState(path, beijingDate(2026, 9, 9, 8, 2, 0))
	if err != nil {
		t.Fatal(err)
	}
	if got.Date != want.Date || !got.Sent["2026-09-09 | 08:00-12:00 | 紫莲刚玉"] {
		t.Fatalf("loaded state = %#v", got)
	}
}

func TestWindowsReplaceRestoresOriginalWhenBackupCleanupFails(t *testing.T) {
	target, tempPath := notificationStateReplacementFiles(t)
	cleanupCalls := 0
	backupPath := ""
	cleanupErr := errors.New("injected backup cleanup failure")
	ops := notificationStateFileOps{
		createTemp: os.CreateTemp,
		lstat:      os.Lstat,
		remove: func(path string) error {
			cleanupCalls++
			if cleanupCalls == 2 {
				return cleanupErr
			}
			return os.Remove(path)
		},
		rename: windowsRenameForTest(target, &backupPath),
	}

	err := replaceNotificationStateFileWindowsWithOps(tempPath, target, ops)
	if !errors.Is(err, cleanupErr) {
		t.Fatalf("error = %v, want cleanup error", err)
	}
	assertNotificationStateFile(t, target, "old")
}

func TestWindowsReplaceRestoresOriginalWhenNewTargetVerificationFails(t *testing.T) {
	target, tempPath := notificationStateReplacementFiles(t)
	backupPath := ""
	verificationErr := errors.New("injected new target verification failure")
	installed := false
	verificationCalls := 0
	ops := notificationStateFileOps{
		createTemp: os.CreateTemp,
		lstat: func(path string) (os.FileInfo, error) {
			if path == target && installed {
				verificationCalls++
				if verificationCalls == 1 {
					return nil, verificationErr
				}
			}
			return os.Lstat(path)
		},
		remove: os.Remove,
		rename: func(oldPath, newPath string) error {
			if oldPath == target {
				backupPath = newPath
			}
			if oldPath == tempPath && newPath == target {
				installed = true
			}
			return windowsRenameForTest(target, &backupPath)(oldPath, newPath)
		},
	}

	err := replaceNotificationStateFileWindowsWithOps(tempPath, target, ops)
	if !errors.Is(err, verificationErr) {
		t.Fatalf("error = %v, want verification error", err)
	}
	assertNotificationStateFile(t, target, "old")
}

func TestWindowsReplaceRestoresOriginalWhenInstallFails(t *testing.T) {
	target, tempPath := notificationStateReplacementFiles(t)
	backupPath := ""
	installErr := errors.New("injected install failure")
	ops := notificationStateFileOps{
		createTemp: os.CreateTemp,
		lstat:      os.Lstat,
		remove:     os.Remove,
		rename: func(oldPath, newPath string) error {
			if oldPath == tempPath && newPath == target {
				return installErr
			}
			return windowsRenameForTest(target, &backupPath)(oldPath, newPath)
		},
	}

	err := replaceNotificationStateFileWindowsWithOps(tempPath, target, ops)
	if !errors.Is(err, installErr) {
		t.Fatalf("error = %v, want install error", err)
	}
	assertNotificationStateFile(t, target, "old")
}

func notificationStateReplacementFiles(t *testing.T) (string, string) {
	t.Helper()
	dir := t.TempDir()
	target := filepath.Join(dir, "state.json")
	tempPath := filepath.Join(dir, "state.json.tmp")
	if err := os.WriteFile(target, []byte("old"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(tempPath, []byte("new"), 0o600); err != nil {
		t.Fatal(err)
	}
	return target, tempPath
}

func windowsRenameForTest(target string, backupPath *string) func(string, string) error {
	return func(oldPath, newPath string) error {
		if oldPath == target && newPath != target && *backupPath == "" {
			*backupPath = newPath
		}
		if oldPath == *backupPath && newPath == target {
			if _, err := os.Lstat(target); err == nil {
				return errors.New("Windows rename cannot replace an existing target")
			}
		}
		return os.Rename(oldPath, newPath)
	}
}

func assertNotificationStateFile(t *testing.T, path, want string) {
	t.Helper()
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != want {
		t.Fatalf("%s = %q, want %q", path, got, want)
	}
}
