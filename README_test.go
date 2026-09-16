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

// This contract prevents the recommended external scheduler guide from silently
// losing the guardrails that keep the GitHub Actions workflow authoritative.
func TestReadmeRecommendsFreeCloudflareWorkerSchedulerContract(t *testing.T) {
	content, err := os.ReadFile("README.md")
	if err != nil {
		t.Fatalf("read README: %v", err)
	}
	readme := string(content)

	required := []string{
		"建议：使用 Cloudflare Worker 提高调度可靠性",
		"建议为自己的 Fork 配置此方案",
		"整套流程可以零付费运行",
		"公开仓库",
		"standard GitHub-hosted runner",
		"Cloudflare Workers Free",
		"免费额度",
		"Cloudflare Worker 只负责触发和监控",
		"Go 抓取、通知和 GitHub Pages 仍由 GitHub Actions 完成",
		"Start with Hello World",
		"不要选择 Import a repository",
		"fine-grained personal access token",
		"只选择自己的 Fork",
		"Actions: Read and write",
		"GITHUB_TOKEN",
		"Cloudflare Secret",
		"不要写入 Worker 源码",
		"const OWNER = \"<你的 GitHub 用户名>\";",
		"const REPO = \"<你的 Fork 仓库名>\";",
		"2-59/10 0-15 * * *",
		"9-59/10 0-15 * * *",
		"actions/workflows/update.yml/dispatches",
		"ref: \"main\"",
		"ACTIVE_STATUSES",
		"STALE_AFTER_MS = 5 * 60 * 1000",
		"nowMs - startedAt <= STALE_AFTER_MS",
		"/actions/runs/${run.id}/cancel",
		"cancelResponse.status !== 409",
		"cancellationFailures.push",
		"GitHub workflow cancellations failed",
		"不补跑",
		"Settings → Cron Triggers",
		"真正注册计划",
		"代码中的 cron 常量只用于分流",
		"GitHub 内置 schedule 仍作为备用",
		"额外运行",
		"去重状态",
		"重复通知",
		"只有成功写回 `gh-pages/state.json` 后",
		"后续串行运行通常才不会重复",
		"通知已送达但状态发布前失败或被取消",
		"仍可能重复发送并影响额度",
		"按事件类型确认",
		"Worker 只产生一个 `workflow_dispatch`",
		"同一分钟可能另有 `schedule` 运行",
		"Token 轮换或撤销",
		"Cloudflare 的免费额度和安全规则以官方当前规则为准",
	}
	for _, want := range required {
		if !strings.Contains(readme, want) {
			t.Errorf("README is missing recommended Worker scheduler guidance %q", want)
		}
	}

	if strings.Contains(readme, "`gh-pages/state.json` 中的去重状态会避免对已成功发送的商品重复通知") {
		t.Error("README makes an unconditional state-deduplication promise")
	}
}

// This test fails if the feature summary promises unconditional notification
// deduplication despite the documented delivery-before-state-write window.
func TestReadmeDoesNotPromiseUnconditionalNotificationDeduplication(t *testing.T) {
	content, err := os.ReadFile("README.md")
	if err != nil {
		t.Fatalf("read README: %v", err)
	}
	if strings.Contains(string(content), "备用运行不会重复推送已经成功发送的商品") {
		t.Fatal("README feature summary makes an unconditional deduplication promise")
	}
}
