package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

var staticFileNames = []string{"index.html", "products.json", "onsale.json", "state.json"}

type fileOps interface {
	Rename(oldPath, newPath string) error
	Remove(path string) error
}

type osFileOps struct{}

func (osFileOps) Rename(oldPath, newPath string) error { return os.Rename(oldPath, newPath) }
func (osFileOps) Remove(path string) error             { return os.Remove(path) }

// WriteStaticOutput transactionally publishes the complete Pages snapshot.
func WriteStaticOutput(outputDir string, result CrawlResult, state NotificationState) error {
	return writeStaticOutputWithOps(outputDir, result, state, osFileOps{})
}

func writeStaticOutputWithOps(outputDir string, result CrawlResult, state NotificationState, ops fileOps) error {
	outputDir = filepath.Clean(outputDir)
	if err := ensureOutputDirectory(outputDir); err != nil {
		return err
	}

	transactionDir, err := os.MkdirTemp(filepath.Dir(outputDir), "."+filepath.Base(outputDir)+".publish-*")
	if err != nil {
		return fmt.Errorf("create static output transaction: %w", err)
	}
	defer func() { _ = os.RemoveAll(transactionDir) }()

	nextDir := filepath.Join(transactionDir, "next")
	backupDir := filepath.Join(transactionDir, "backup")
	if err := os.Mkdir(nextDir, 0o755); err != nil {
		return fmt.Errorf("create next static output directory: %w", err)
	}
	if err := os.Mkdir(backupDir, 0o755); err != nil {
		return fmt.Errorf("create static output backup directory: %w", err)
	}
	if err := generateStaticFiles(nextDir, result, state); err != nil {
		return err
	}

	backedUp := make([]string, 0, len(staticFileNames))
	for _, name := range staticFileNames {
		targetPath := filepath.Join(outputDir, name)
		if _, err := os.Lstat(targetPath); os.IsNotExist(err) {
			continue
		} else if err != nil {
			rollbackErr := rollbackStaticOutput(outputDir, nextDir, backupDir, nil, backedUp, ops)
			return joinStaticOutputErrors(fmt.Errorf("inspect old %s: %w", name, err), rollbackErr)
		}
		if err := ops.Rename(targetPath, filepath.Join(backupDir, name)); err != nil {
			rollbackErr := rollbackStaticOutput(outputDir, nextDir, backupDir, nil, backedUp, ops)
			return joinStaticOutputErrors(fmt.Errorf("back up %s: %w", name, err), rollbackErr)
		}
		backedUp = append(backedUp, name)
	}

	installed := make([]string, 0, len(staticFileNames))
	for _, name := range staticFileNames {
		if err := ops.Rename(filepath.Join(nextDir, name), filepath.Join(outputDir, name)); err != nil {
			rollbackErr := rollbackStaticOutput(outputDir, nextDir, backupDir, installed, backedUp, ops)
			return joinStaticOutputErrors(fmt.Errorf("install %s: %w", name, err), rollbackErr)
		}
		installed = append(installed, name)
	}
	return nil
}

func ensureOutputDirectory(outputDir string) error {
	info, err := os.Stat(outputDir)
	if os.IsNotExist(err) {
		if err := os.MkdirAll(outputDir, 0o755); err != nil {
			return fmt.Errorf("create static output directory: %w", err)
		}
		return nil
	}
	if err != nil {
		return fmt.Errorf("inspect static output directory: %w", err)
	}
	if !info.IsDir() {
		return fmt.Errorf("static output path %q is not a directory", outputDir)
	}
	return nil
}

func generateStaticFiles(dir string, result CrawlResult, state NotificationState) error {
	var page bytes.Buffer
	if err := RenderPage(&page, result); err != nil {
		return fmt.Errorf("render index.html: %w", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "index.html"), page.Bytes(), 0o644); err != nil {
		return fmt.Errorf("write index.html: %w", err)
	}

	if state.Sent == nil {
		state.Sent = make(map[string]bool)
	}
	jsonFiles := []struct {
		name  string
		value any
	}{
		{name: "products.json", value: productsAPIResponse(result)},
		{name: "onsale.json", value: onSaleAPIResponse(result)},
		{name: "state.json", value: state},
	}
	for _, file := range jsonFiles {
		if err := writeAndValidateJSON(filepath.Join(dir, file.name), file.value); err != nil {
			return fmt.Errorf("generate %s: %w", file.name, err)
		}
	}
	return nil
}

func writeAndValidateJSON(path string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal JSON: %w", err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("write JSON: %w", err)
	}
	written, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read JSON for validation: %w", err)
	}
	var decoded any
	if err := json.Unmarshal(written, &decoded); err != nil {
		return fmt.Errorf("validate JSON: %w", err)
	}
	return nil
}

func rollbackStaticOutput(outputDir, nextDir, backupDir string, installed, backedUp []string, ops fileOps) error {
	var rollbackErrors []error
	for i := len(installed) - 1; i >= 0; i-- {
		name := installed[i]
		targetPath := filepath.Join(outputDir, name)
		if err := ops.Remove(targetPath); err != nil && !os.IsNotExist(err) {
			asidePath := filepath.Join(nextDir, ".rollback-"+name)
			if moveErr := ops.Rename(targetPath, asidePath); moveErr != nil {
				rollbackErrors = append(rollbackErrors, fmt.Errorf("remove replacement %s: %v; move it aside: %w", name, err, moveErr))
			}
		}
	}
	for _, name := range backedUp {
		if err := ops.Rename(filepath.Join(backupDir, name), filepath.Join(outputDir, name)); err != nil {
			rollbackErrors = append(rollbackErrors, fmt.Errorf("restore %s: %w", name, err))
		}
	}
	return errors.Join(rollbackErrors...)
}

func joinStaticOutputErrors(operationErr, rollbackErr error) error {
	if rollbackErr == nil {
		return operationErr
	}
	return errors.Join(operationErr, fmt.Errorf("rollback static output: %w", rollbackErr))
}

func productsAPIResponse(result CrawlResult) APIResponse {
	return APIResponse{Code: 200, Message: "success", Data: result}
}

func onSaleAPIResponse(result CrawlResult) APIResponse {
	products := make([]Product, 0, result.OnSaleCount)
	for _, product := range result.Products {
		if product.IsOnSale {
			products = append(products, product)
		}
	}
	return APIResponse{
		Code:    200,
		Message: "success",
		Data: map[string]any{
			"time_slots":    result.TimeSlots,
			"products":      products,
			"on_sale_count": len(products),
			"updated_at":    result.UpdatedAt,
		},
	}
}
