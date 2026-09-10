package main

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

// This test fails if a release drops any instruction needed to run locally or
// deploy a fork without a server.
func TestReadmeDeploymentContract(t *testing.T) {
	content, err := os.ReadFile("README.md")
	if err != nil {
		t.Fatalf("read README: %v", err)
	}
	readme := string(content)

	required := []string{
		"go run . -once -output ./public -state ./public/state.json",
		"Fork 本仓库",
		"自己的 Fork",
		"SERVERCHAN_SENDKEY",
		"workflow_dispatch",
		"gh-pages",
		"GitHub Pages",
		"08:02 至 23:52",
		"每 10 分钟",
		"每天有 96 次检查",
		"60 天",
		"standard GitHub-hosted runners",
		"5 条/天",
		"Server酱 Turbo",
		"SCT",
		"微信",
		"Server酱³",
		"sctp",
		"App",
		"https://github.com/kanzakine/Roco-API",
		"未复制",
	}
	for _, want := range required {
		if !strings.Contains(readme, want) {
			t.Errorf("README is missing %q", want)
		}
	}

	for _, unwanted := range []string{
		"部署到新的独立公开仓库",
		"git remote add publish",
		"git push publish main:main",
	} {
		if strings.Contains(readme, unwanted) {
			t.Errorf("README still contains obsolete separate-repository guidance %q", unwanted)
		}
	}

	standaloneRun := regexp.MustCompile("(?m)^```bash\\r?\\ngo run \\.\\r?\\n```$")
	if !standaloneRun.MatchString(readme) {
		t.Error("README is missing an exact standalone `go run .` command block")
	}
}

// This test fails if deployment guidance asks users to grant write access to
// every workflow instead of relying on update.yml's scoped contents grant.
func TestReadmeDocumentsWorkflowScopedLeastPrivilege(t *testing.T) {
	content, err := os.ReadFile("README.md")
	if err != nil {
		t.Fatalf("read README: %v", err)
	}
	readme := string(content)

	if strings.Contains(readme, "Read and write permissions") {
		t.Fatal("README recommends repository-wide read/write workflow permissions")
	}
	for _, want := range []string{"permissions: contents: write", "无需把整个仓库的默认 Workflow permissions 调整为读写"} {
		if !strings.Contains(readme, want) {
			t.Errorf("README is missing least-privilege guidance %q", want)
		}
	}
}
