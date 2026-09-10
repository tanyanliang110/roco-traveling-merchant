package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"time"
)

var beijingLocation = time.FixedZone("CST", 8*60*60)

// NotificationState records products that have already been sent on one day.
type NotificationState struct {
	Date string          `json:"date"`
	Sent map[string]bool `json:"sent"`
}

// LoadNotificationState loads today's sent-product state. A missing state file
// starts an empty state, while a state from an earlier day is reset.
func LoadNotificationState(path string, today time.Time) (NotificationState, error) {
	date := notificationDate(today)
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return newNotificationState(date), nil
	}
	if err != nil {
		return NotificationState{}, fmt.Errorf("read notification state: %w", err)
	}

	var state NotificationState
	if err := json.Unmarshal(data, &state); err != nil {
		return NotificationState{}, fmt.Errorf("parse notification state: %w", err)
	}
	if state.Date != date {
		return newNotificationState(date), nil
	}
	if state.Sent == nil {
		state.Sent = make(map[string]bool)
	}
	return state, nil
}

// SaveNotificationState writes state atomically by replacing the target with a
// fully written temporary file from the same directory.
func SaveNotificationState(path string, state NotificationState) error {
	if state.Sent == nil {
		state.Sent = make(map[string]bool)
	}

	dir := filepath.Dir(path)
	temp, err := os.CreateTemp(dir, "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return fmt.Errorf("create notification state temporary file: %w", err)
	}
	tempPath := temp.Name()
	cleanupTemp := true
	defer func() {
		if cleanupTemp {
			_ = os.Remove(tempPath)
		}
	}()

	if err := json.NewEncoder(temp).Encode(state); err != nil {
		_ = temp.Close()
		return fmt.Errorf("write notification state: %w", err)
	}
	if err := temp.Sync(); err != nil {
		_ = temp.Close()
		return fmt.Errorf("sync notification state: %w", err)
	}
	if err := temp.Close(); err != nil {
		return fmt.Errorf("close notification state: %w", err)
	}
	if err := replaceNotificationStateFile(tempPath, path); err != nil {
		return err
	}
	cleanupTemp = false
	return nil
}

// NotificationKey identifies a product within its Beijing-calendar-day slot.
func NotificationKey(product Product, now time.Time) string {
	return fmt.Sprintf("%s | %s | %s", notificationDate(now), product.SlotLabel, product.Name)
}

// Pending returns products that have not been sent in the supplied Beijing day.
func (s NotificationState) Pending(products []Product, now time.Time) []Product {
	if s.Date != notificationDate(now) {
		return append([]Product(nil), products...)
	}

	pending := make([]Product, 0, len(products))
	for _, product := range products {
		if !s.Sent[NotificationKey(product, now)] {
			pending = append(pending, product)
		}
	}
	return pending
}

// MarkSent records products as sent for the supplied Beijing day.
func (s *NotificationState) MarkSent(products []Product, now time.Time) {
	date := notificationDate(now)
	if s.Date != date || s.Sent == nil {
		s.Date = date
		s.Sent = make(map[string]bool)
	}
	for _, product := range products {
		s.Sent[NotificationKey(product, now)] = true
	}
}

func newNotificationState(date string) NotificationState {
	return NotificationState{Date: date, Sent: make(map[string]bool)}
}

func notificationDate(now time.Time) string {
	return now.In(beijingLocation).Format("2006-01-02")
}

func replaceNotificationStateFile(tempPath, path string) error {
	if runtime.GOOS != "windows" {
		if err := os.Rename(tempPath, path); err != nil {
			return fmt.Errorf("replace notification state: %w", err)
		}
		return nil
	}
	return replaceNotificationStateFileWindows(tempPath, path)
}

// replaceNotificationStateFileWindows avoids Windows' inability to rename over
// an existing file. The old file is first moved to a unique backup and restored
// if installing the new file fails.
func replaceNotificationStateFileWindows(tempPath, path string) error {
	return replaceNotificationStateFileWindowsWithOps(tempPath, path, notificationStateOSFileOps())
}

type notificationStateFileOps struct {
	createTemp func(dir, pattern string) (*os.File, error)
	lstat      func(name string) (os.FileInfo, error)
	rename     func(oldPath, newPath string) error
	remove     func(name string) error
}

func notificationStateOSFileOps() notificationStateFileOps {
	return notificationStateFileOps{
		createTemp: os.CreateTemp,
		lstat:      os.Lstat,
		rename:     os.Rename,
		remove:     os.Remove,
	}
}

func replaceNotificationStateFileWindowsWithOps(tempPath, path string, ops notificationStateFileOps) error {
	if _, err := ops.lstat(path); os.IsNotExist(err) {
		if err := ops.rename(tempPath, path); err != nil {
			return fmt.Errorf("create notification state: %w", err)
		}
		return nil
	} else if err != nil {
		return fmt.Errorf("inspect notification state: %w", err)
	}

	backup, err := ops.createTemp(filepath.Dir(path), "."+filepath.Base(path)+".backup-*")
	if err != nil {
		return fmt.Errorf("create notification state backup: %w", err)
	}
	backupPath := backup.Name()
	if err := backup.Close(); err != nil {
		_ = ops.remove(backupPath)
		return fmt.Errorf("close notification state backup: %w", err)
	}
	if err := ops.remove(backupPath); err != nil {
		return fmt.Errorf("prepare notification state backup: %w", err)
	}
	if err := ops.rename(path, backupPath); err != nil {
		return fmt.Errorf("back up notification state: %w", err)
	}
	if _, err := ops.lstat(backupPath); err != nil {
		return rollbackNotificationStateBackup(ops, tempPath, backupPath, path, fmt.Errorf("verify notification state backup: %w", err))
	}
	if err := ops.rename(tempPath, path); err != nil {
		return rollbackNotificationStateBackup(ops, tempPath, backupPath, path, fmt.Errorf("replace notification state: %w", err))
	}
	if _, err := ops.lstat(path); err != nil {
		return rollbackNotificationStateBackup(ops, tempPath, backupPath, path, fmt.Errorf("verify replaced notification state: %w", err))
	}
	if err := ops.remove(backupPath); err != nil {
		return rollbackNotificationStateBackup(ops, tempPath, backupPath, path, fmt.Errorf("remove notification state backup: %w", err))
	}
	return nil
}

func rollbackNotificationStateBackup(ops notificationStateFileOps, tempPath, backupPath, path string, replaceErr error) error {
	if _, err := ops.lstat(path); err == nil {
		if err := ops.rename(path, tempPath); err != nil {
			return fmt.Errorf("%v; move replacement aside for notification state rollback: %w", replaceErr, err)
		}
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("%v; inspect notification state before rollback: %w", replaceErr, err)
	}
	if err := ops.rename(backupPath, path); err != nil {
		return fmt.Errorf("%v; restore notification state backup: %w", replaceErr, err)
	}
	return replaceErr
}
