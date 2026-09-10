package main

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

var staticOutputNames = []string{"index.html", "products.json", "onsale.json", "state.json"}

// This test fails if a successful publish omits any member of the public
// four-file snapshot.
func TestWriteStaticOutputCreatesExpectedFiles(t *testing.T) {
	dir := t.TempDir()
	if err := WriteStaticOutput(dir, pageTestResult(), testState()); err != nil {
		t.Fatal(err)
	}
	for _, name := range staticOutputNames {
		if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
	}
}

// This test fails if the static JSON contracts diverge from their local API
// counterparts or if onsale.json includes a non-sale product.
func TestStaticJSONMatchesAPIRoutesAndFiltersOnSale(t *testing.T) {
	result := pageTestResult()
	dir := t.TempDir()
	if err := WriteStaticOutput(dir, result, testState()); err != nil {
		t.Fatal(err)
	}

	handler := newServerWithResult(result).Handler()
	for _, item := range []struct {
		name string
		path string
	}{
		{name: "products.json", path: "/api/products"},
		{name: "onsale.json", path: "/api/onsale"},
	} {
		fileValue := decodeJSONFile(t, filepath.Join(dir, item.name))
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, item.path, nil))
		var apiValue any
		if err := json.Unmarshal(recorder.Body.Bytes(), &apiValue); err != nil {
			t.Fatalf("decode %s response: %v", item.path, err)
		}
		if !reflect.DeepEqual(fileValue, apiValue) {
			t.Fatalf("%s differs from %s\nfile: %#v\napi:  %#v", item.name, item.path, fileValue, apiValue)
		}
	}

	onsale := decodeJSONFile(t, filepath.Join(dir, "onsale.json")).(map[string]any)
	data := onsale["data"].(map[string]any)
	products := data["products"].([]any)
	if len(products) != 1 || products[0].(map[string]any)["name"] != "在售商品" {
		t.Fatalf("onsale products = %#v", products)
	}
}

// This test fails if public JSON field names drift from the documented API or
// if the exact browser countdown boundaries are omitted or altered.
func TestProductsJSONUsesExactEnvelopeAndProductKeys(t *testing.T) {
	dir := t.TempDir()
	if err := WriteStaticOutput(dir, pageTestResult(), testState()); err != nil {
		t.Fatal(err)
	}

	envelope := decodeJSONFile(t, filepath.Join(dir, "products.json")).(map[string]any)
	assertExactJSONKeys(t, "envelope", envelope, "code", "message", "data")
	data := envelope["data"].(map[string]any)
	assertExactJSONKeys(t, "data", data, "time_slots", "products", "on_sale_count", "total_count", "updated_at")
	timeSlot := data["time_slots"].([]any)[0].(map[string]any)
	assertExactJSONKeys(t, "time slot", timeSlot, "label")
	product := data["products"].([]any)[0].(map[string]any)
	assertExactJSONKeys(t, "product", product,
		"name", "price", "limit", "category", "desc", "image_url",
		"is_on_sale", "has_ended", "is_upcoming", "remain", "slot_label",
		"start_at", "end_at",
	)
	if product["start_at"] != float64(1788936000) || product["end_at"] != float64(1788950400) {
		t.Fatalf("JSON boundaries = %#v/%#v, want 1788936000/1788950400", product["start_at"], product["end_at"])
	}
}

// This test fails if deployment state gains configuration or secret fields
// instead of serializing only NotificationState's public persistence contract.
func TestStaticStateContainsNoSecretConfiguration(t *testing.T) {
	dir := t.TempDir()
	state := testState()
	if err := WriteStaticOutput(dir, pageTestResult(), state); err != nil {
		t.Fatal(err)
	}

	raw, err := os.ReadFile(filepath.Join(dir, "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(strings.ToLower(string(raw)), "sendkey") {
		t.Fatalf("state contains secret configuration field: %s", raw)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		t.Fatal(err)
	}
	if len(fields) != 2 || fields["date"] == nil || fields["sent"] == nil {
		t.Fatalf("state fields = %v, want only date and sent", reflect.ValueOf(fields).MapKeys())
	}
	var got NotificationState
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, state) {
		t.Fatalf("state = %#v, want %#v", got, state)
	}
}

// This test fails if an install error can expose a mixed generation or discard
// any member of the previous complete snapshot.
func TestWriteStaticOutputRollsBackAllFilesWhenSecondReplacementFails(t *testing.T) {
	dir := t.TempDir()
	old := make(map[string][]byte, len(staticOutputNames))
	for _, name := range staticOutputNames {
		old[name] = []byte("old-" + name + "\n")
		if err := os.WriteFile(filepath.Join(dir, name), old[name], 0o644); err != nil {
			t.Fatal(err)
		}
	}

	wantErr := errors.New("replace second file")
	ops := &failSecondReplacementOps{outputDir: dir, failure: wantErr}
	err := writeStaticOutputWithOps(dir, pageTestResult(), testState(), ops)
	if !errors.Is(err, wantErr) {
		t.Fatalf("error = %v, want %v", err, wantErr)
	}
	if ops.replacements != 2 {
		t.Fatalf("replacement attempts = %d, want 2", ops.replacements)
	}
	for _, name := range staticOutputNames {
		got, readErr := os.ReadFile(filepath.Join(dir, name))
		if readErr != nil {
			t.Fatalf("read restored %s: %v", name, readErr)
		}
		if !reflect.DeepEqual(got, old[name]) {
			t.Fatalf("%s = %q after rollback, want %q", name, got, old[name])
		}
	}
}

// This test fails if publishing treats the Pages checkout as disposable and
// removes its Git metadata or unrelated assets.
func TestWriteStaticOutputPreservesUnrelatedFilesAndGitMetadata(t *testing.T) {
	dir := t.TempDir()
	preserved := map[string][]byte{
		filepath.Join(dir, ".git"):  []byte("gitdir: /repo/.git/workbook/pages\n"),
		filepath.Join(dir, "CNAME"): []byte("roco.example.test\n"),
	}
	for path, content := range preserved {
		if err := os.WriteFile(path, content, 0o644); err != nil {
			t.Fatal(err)
		}
	}

	if err := WriteStaticOutput(dir, pageTestResult(), testState()); err != nil {
		t.Fatal(err)
	}
	for path, want := range preserved {
		got, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read preserved %s: %v", path, err)
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("preserved %s = %q, want %q", path, got, want)
		}
	}
}

type failSecondReplacementOps struct {
	outputDir    string
	failure      error
	replacements int
	failed       bool
}

func (f *failSecondReplacementOps) Rename(oldPath, newPath string) error {
	if !f.failed && filepath.Clean(filepath.Dir(newPath)) == filepath.Clean(f.outputDir) && isStaticOutputName(filepath.Base(newPath)) {
		f.replacements++
		if f.replacements == 2 {
			f.failed = true
			return f.failure
		}
	}
	return os.Rename(oldPath, newPath)
}

func (f *failSecondReplacementOps) Remove(path string) error {
	return os.Remove(path)
}

func isStaticOutputName(name string) bool {
	for _, candidate := range staticOutputNames {
		if name == candidate {
			return true
		}
	}
	return false
}

func decodeJSONFile(t *testing.T, path string) any {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		t.Fatalf("decode %s: %v", path, err)
	}
	return value
}

func assertExactJSONKeys(t *testing.T, label string, value map[string]any, keys ...string) {
	t.Helper()
	want := make(map[string]bool, len(keys))
	for _, key := range keys {
		want[key] = true
	}
	if len(value) != len(want) {
		t.Fatalf("%s keys = %v, want exactly %v", label, reflect.ValueOf(value).MapKeys(), keys)
	}
	for key := range value {
		if !want[key] {
			t.Fatalf("%s contains unexpected key %q; want exactly %v", label, key, keys)
		}
	}
}
