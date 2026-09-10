package main

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

func TestUpdateWorkflowContract(t *testing.T) {
	content, err := os.ReadFile(".github/workflows/update.yml")
	if err != nil {
		t.Fatalf("read update workflow: %v", err)
	}
	workflow := string(content)

	required := []string{
		"workflow_dispatch:",
		"cron: '2-59/10 0-15 * * *'",
		"contents: write",
		"group: roco-pages-update",
		"cancel-in-progress: false",
		"uses: actions/checkout@v7",
		"uses: actions/setup-go@v7",
		"go-version-file: go.mod",
		"go test ./...",
		"SERVERCHAN_SENDKEY: ${{ secrets.SERVERCHAN_SENDKEY }}",
		`go run . -once -output "$GITHUB_WORKSPACE/pages" -state "$GITHUB_WORKSPACE/pages/state.json"`,
		"git add -- index.html products.json onsale.json state.json",
		"git diff --cached --quiet",
		"git push origin HEAD:gh-pages",
	}
	for _, want := range required {
		if !strings.Contains(workflow, want) {
			t.Errorf("workflow is missing %q", want)
		}
	}

	testAt := strings.Index(workflow, "go test ./...")
	publishAt := strings.Index(workflow, `go run . -once -output "$GITHUB_WORKSPACE/pages"`)
	if testAt < 0 || publishAt < 0 || testAt > publishAt {
		t.Error("workflow must test the main checkout before publishing")
	}

	if got := strings.Count(workflow, "${{ secrets.SERVERCHAN_SENDKEY }}"); got != 1 {
		t.Errorf("secret expression count = %d, want exactly one env mapping", got)
	}
	forbidden := regexp.MustCompile(`(?i)SCT[A-Za-z0-9]+|sctp[0-9]+t[A-Za-z0-9]+`)
	if match := forbidden.FindString(workflow); match != "" {
		t.Errorf("workflow contains a send-key-shaped value %q", match)
	}
	if strings.Contains(workflow, "git add .") || strings.Contains(workflow, "git add -A") {
		t.Error("workflow must stage only the four generated files")
	}
}
