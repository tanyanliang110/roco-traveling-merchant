# Roco-API 🏪

洛克王国：世界每日远行商人商品查询。项目既能作为本地常驻 HTTP 服务运行，也能由 GitHub Actions 定时执行一次，并把页面和 JSON 发布到 GitHub Pages，全程不需要自有服务器。

数据来源：[快爆工具箱 - 远行商人查询器](https://www.onebiji.com/hykb_tools/comm/lkwgmerchant/preview.php?id=1&immgj=0)。

## 功能

- 抓取商品名称、价格、限购、分类、描述、图片和销售时段。
- 保留本地 Web 页面以及 `/api/products`、`/api/onsale` 两个 JSON API。
- 单次生成 `index.html`、`products.json`、`onsale.json`、`state.json`，适合 GitHub Pages。
- 按“北京时间日期 + 时段 + 商品名”持久化推送状态，备用运行不会重复推送已经成功发送的商品。
- 支持 Server酱 Turbo、Server酱³，以及兼容用的完整 HTTP/HTTPS 推送地址。

## 本地常驻服务

需要 [Go](https://go.dev/dl/) 1.26 或更高版本。`config.json` 是可选的；文件不存在时会使用 `:8008`、3 分钟抓取间隔和项目内置的数据源。

如需修改配置，先复制不含 secret 的示例：

```bash
cp config.example.json config.json
```

`config.json` 已被 Git 忽略。`serverchan.uid` 仅为旧配置兼容字段；当前端点可直接由 `serverchan.sendkey` 推导，不需要单独填写 UID。

启动常驻服务：

```bash
go run .
```

启动后可访问：

- `http://localhost:8008/`：Web 状态页。
- `http://localhost:8008/api/products`：全部商品。
- `http://localhost:8008/api/onsale`：当前在售商品。

常驻服务会先抓取一次，然后按照 `crawl.interval` 的分钟数刷新。后续抓取失败时保留上一次成功的内存数据。

## 本地单次模式

以下是本地生成四个静态文件的完整命令；`state.json` 必须位于输出目录中：

```bash
go run . -once -output ./public -state ./public/state.json
```

单次模式抓取、按需推送、原子更新四个文件后退出。抓取、解析、推送或写入失败时返回非零退出码，且不会用不完整结果覆盖上一版输出。未配置 SendKey 时仍会生成文件，并明确记录 `推送未启用`；这不会把商品标成已推送。

如需在本机临时启用推送，可通过环境变量提供凭证。请把占位文本替换为自己的值，不要把真实值写进 README、脚本、日志或已跟踪文件：

```bash
SERVERCHAN_SENDKEY='<你的 SendKey>' go run . -once -output ./public -state ./public/state.json
```

## Server酱选择

只需配置一个 `SERVERCHAN_SENDKEY`：

| 服务           | SendKey 格式     | 消息接收位置            |
| -------------- | ---------------- | ----------------------- |
| Server酱 Turbo | `SCT…`        | 微信等 Turbo 已配置通道 |
| Server酱³     | `sctp<uid>t…` | Server酱³ App          |

两个版本的 SendKey 不通用。Server酱³ 的 UID 会从 `sctp` 和 `t` 之间自动解析。获取和核对格式时以 [Server酱官方 SendKey 文档](https://sct.ftqq.com/docs/getting-started/sendkey/) 为准。

## Fork 本仓库并部署

以下步骤适合希望在自己 GitHub 账号中运行定时抓取、接收通知并发布页面的使用者。无需另建仓库或复制代码；Fork 本仓库后，所有 Secret、Actions 运行记录和 Pages 内容都由你自己的 Fork 管理。

### 1. 创建并启用自己的 Fork

1. 登录 GitHub，打开本仓库页面，点击右上角 **Fork**，选择自己的账号并完成创建。可以保留原仓库名，也可以为 Fork 改名；记下你的 `<用户名>` 和 `<Fork 仓库名>`。
2. 打开自己的 Fork，确认默认分支为 `main`，并且 `.github/workflows/update.yml` 已存在。
3. 进入 Fork 的 **Actions** 页面；如果 GitHub 提示 Fork 中的工作流尚未启用，选择 **I understand my workflows, go ahead and enable them**。启用前请先阅读工作流内容。
4. `.github/workflows/update.yml` 已在工作流级声明 `permissions: contents: write`，这已足够更新 `gh-pages`；无需把整个仓库的默认 Workflow permissions 调整为读写，应继续保持只读。

### 2. 添加 Repository Secret

1. 打开 **Settings → Secrets and variables → Actions**。
2. 选择 **New repository secret**。
3. Name 必须填写 `SERVERCHAN_SENDKEY`，Secret 填写你自己的 Turbo 或 Server酱³ SendKey，然后保存。

Repository Secret 只会映射到生成步骤的环境变量。不要把真实 SendKey 填入 `config.example.json`、提交到 Git，或粘贴到 issue/Actions 日志。若不需要通知，可以不创建这个 Secret；页面仍会更新，日志会显示 `推送未启用`。

### 3. 手动完成首次运行

打开 **Actions → Update static pages**，选择 **Run workflow**，分支选择 `main` 后确认运行。这是工作流的 `workflow_dispatch` 入口。第一次成功运行会创建 `gh-pages` 分支；先等待它成功，再设置 Pages。若运行失败，请在 Actions 日志中处理原因后重新手动运行，不要先选择一个尚不存在的 Pages 分支。

### 4. 启用 GitHub Pages

在 **Settings → Pages → Build and deployment** 中设置：

- Source：**Deploy from a branch**。
- Branch：`gh-pages`。
- Folder：`/(root)`。

保存后，项目站点 URL 通常是：

```text
https://<用户名>.github.io/<Fork 仓库名>/
```

两个 JSON 地址相应为 `https://<用户名>.github.io/<Fork 仓库名>/products.json` 和 `https://<用户名>.github.io/<Fork 仓库名>/onsale.json`。如果 Fork 仓库名本身是 `<用户名>.github.io`，站点使用用户站点根地址。GitHub 的当前界面和规则请参考[从分支配置 Pages 发布源](https://docs.github.com/en/pages/getting-started-with-github-pages/configuring-a-publishing-source-for-your-github-pages-site)。

## 定时运行和备用检查

工作流每天北京时间 08:02 至 23:52 每 10 分钟运行一次，00:00 至 08:00 不运行，因为这段时间没有远行商人。对应 UTC cron 是 `2-59/10 0-15 * * *`。

频繁检查用于降低 GitHub Actions 计划任务延迟或丢弃造成的影响。成功状态保存在 `gh-pages/state.json`，正常情况下后续检查不会重复通知已经成功发送的商品。

GitHub 明确说明 `schedule` 在 Actions 高负载时可能延迟，极端情况下排队任务也可能被丢弃，因此这些时间不是秒级 SLA，备用检查也不是绝对保证。详见 [GitHub Actions 的 `schedule` 说明](https://docs.github.com/en/actions/reference/workflows-and-actions/events-that-trigger-workflows#schedule)。

公开仓库若连续 60 天没有仓库活动，GitHub 可能自动禁用定时工作流。发现页面不再更新时，到 Actions 页面重新启用工作流并用 `workflow_dispatch` 手动运行一次。详见 [GitHub 的工作流启用/禁用说明](https://docs.github.com/en/actions/managing-workflow-runs-and-deployments/managing-workflow-runs/disabling-and-enabling-a-workflow)。

### 可选：Cloudflare Worker 可靠调度与自愈

这是为需要额外调度保障的 Fork 使用者准备的**可选**方案；不配置它也能正常使用。Cloudflare Worker 只负责触发和监控，Go 抓取、通知和 GitHub Pages 仍由 GitHub Actions 完成。不要把抓取逻辑、`SERVERCHAN_SENDKEY` 或 Pages 发布逻辑搬到 Worker。

GitHub 内置 schedule 仍作为备用。它和 Worker 同时触发时可能产生额外运行；只有成功写回 `gh-pages/state.json` 后，后续串行运行通常才不会重复对已成功发送的商品通知。这个去重状态不是“至少一次通知”的保证：若通知已送达但状态发布前失败或被取消，仍可能重复发送并影响额度；仍应留意 Actions 的实际运行次数和额度。

#### 1. 为自己的 Fork 创建最小权限 Token

1. 在 GitHub **Settings → Developer settings → Personal access tokens → Fine-grained tokens** 创建一个 `fine-grained personal access token`，设置尽可能短的过期时间。
2. Resource owner 选择自己的账号；Repository access 选择 **Only select repositories**，并且只选择自己的 Fork（不要选择上游仓库或 “All repositories”）。
3. 在 Repository permissions 中把 **Actions: Read and write** 设为允许；不要额外授予不需要的权限。这个权限用于触发工作流和读取/取消卡住的运行。
4. 复制 Token 后立即妥善保存。它只会用作 Worker 的 `GITHUB_TOKEN`，不得提交、粘贴到日志或发送给他人。

#### 2. 创建 Worker 和 Secret

1. 打开 Cloudflare Dashboard，创建 Worker 时选择 **Start with Hello World**，不要选择 Import a repository；本教程使用 Dashboard 中的单文件 Worker，而不是让 Cloudflare 读取仓库源码。
2. 把下面完整示例粘贴到 Worker 编辑器。只编辑 `OWNER` 和 `REPO` 这两个非秘密占位值，分别填入你的 GitHub 用户名和 Fork 仓库名。
3. 在 Worker 的 **Settings → Variables and Secrets** 中添加 Secret：名称为 `GITHUB_TOKEN`，值为刚创建的 PAT，并选择加密/Secret 类型。`GITHUB_TOKEN` 必须是 Cloudflare Secret，不要写入 Worker 源码、常量、README 截图或 GitHub Secret。
4. 保存并部署 Worker。部署不会自动创建 cron；下一步必须在 Dashboard 注册它们。

```js
// 只编辑这两个非秘密值；Token 由 Cloudflare Secret GITHUB_TOKEN 提供。
const OWNER = "<你的 GitHub 用户名>";
const REPO = "<你的 Fork 仓库名>";

export const REGULAR_CRON = "2-59/10 0-15 * * *";
export const WATCHDOG_CRON = "9-59/10 0-15 * * *";
const STALE_AFTER_MS = 5 * 60 * 1000;
const API_ROOT = `https://api.github.com/repos/${OWNER}/${REPO}`;
const ACTIVE_STATUSES = new Set([
  "in_progress",
  "pending",
  "queued",
  "requested",
  "waiting",
]);

function requestHeaders(token) {
  if (!token) throw new Error("Missing Cloudflare Secret GITHUB_TOKEN");
  return {
    Accept: "application/vnd.github+json",
    Authorization: `Bearer ${token}`,
    "Content-Type": "application/json",
    "User-Agent": "roco-cloudflare-scheduler",
    "X-GitHub-Api-Version": "2022-11-28",
  };
}

async function dispatchWorkflow(env, fetchImpl) {
  const response = await fetchImpl(
    `${API_ROOT}/actions/workflows/update.yml/dispatches`,
    {
      method: "POST",
      headers: requestHeaders(env.GITHUB_TOKEN),
      body: JSON.stringify({ ref: "main" }),
    },
  );
  if (!response.ok) {
    throw new Error(`GitHub workflow dispatch failed: ${response.status}`);
  }
}

async function cancelStaleRuns(env, fetchImpl, nowMs) {
  const cancellationFailures = [];
  for (const status of ACTIVE_STATUSES) {
    const response = await fetchImpl(
      `${API_ROOT}/actions/workflows/update.yml/runs?${new URLSearchParams({
        status,
        branch: "main",
        per_page: "100",
      })}`,
      { method: "GET", headers: requestHeaders(env.GITHUB_TOKEN) },
    );
    if (!response.ok) {
      throw new Error(`GitHub list workflow runs failed: ${response.status}`);
    }

    const payload = await response.json();
    const runs = Array.isArray(payload.workflow_runs) ? payload.workflow_runs : [];
    for (const run of runs) {
      if (!ACTIVE_STATUSES.has(run.status)) continue;
      const startedAt = Date.parse(run.run_started_at) || Date.parse(run.created_at);
      // 仅严格超过五分钟才取消；正好五分钟仍保留。
      if (!Number.isFinite(startedAt) || nowMs - startedAt <= STALE_AFTER_MS) {
        continue;
      }

      const cancelResponse = await fetchImpl(
        `${API_ROOT}/actions/runs/${run.id}/cancel`,
        { method: "POST", headers: requestHeaders(env.GITHUB_TOKEN) },
      );
      // 已完成和竞争导致的 409 无害；其他失败逐项汇总，继续处理剩余运行。
      if (!cancelResponse.ok && cancelResponse.status !== 409) {
        cancellationFailures.push(`${run.id}: ${cancelResponse.status}`);
      }
    }
  }
  if (cancellationFailures.length > 0) {
    throw new Error(
      `GitHub workflow cancellations failed: ${cancellationFailures.join(", ")}`,
    );
  }
}

async function handleScheduled(cron, env, fetchImpl = fetch, nowMs = Date.now()) {
  if (cron === REGULAR_CRON) {
    await dispatchWorkflow(env, fetchImpl);
  } else if (cron === WATCHDOG_CRON) {
    // 自愈只取消严格超时的运行，不补跑；下一次正常 cron 会再触发一次。
    await cancelStaleRuns(env, fetchImpl, nowMs);
  }
}

export default {
  async scheduled(controller, env, ctx) {
    ctx.waitUntil(handleScheduled(controller.cron, env));
  },
  fetch() {
    return new Response("Roco scheduler is active.");
  },
};
```

#### 3. 在 Dashboard 真正注册两个计划

到 Worker 的 **Settings → Cron Triggers**，分别添加下面两个 UTC cron（Cloudflare 的界面可能把这一页显示为 Triggers 下的 Cron Triggers）：

| 用途 | Cron | 行为 |
| --- | --- | --- |
| 正常触发 | `2-59/10 0-15 * * *` | 对 `main` 发送 `update.yml` 的 `workflow_dispatch`。 |
| 自愈检查 | `9-59/10 0-15 * * *` | 只检查 `main` 上处于 active status 的运行；严格超过 5 分钟才逐项取消。 |

Settings → Cron Triggers 中的记录才真正注册计划；代码中的 cron 常量只用于分流 `scheduled` 事件，单独部署代码不会让 cron 自动生效。自愈取消接口返回 `409` 时表示竞争中的无害结果；示例会继续检查其他运行，汇总逐项取消失败后报错，且不补跑工作流。

#### 4. 验证、轮换与安全

1. 先在 GitHub Actions 页面手动运行一次 `Update static pages`，确认 Fork、`main` 和 Pages 已按上文配置成功；然后等待下一次 Worker 正常 cron，在 Actions 页面按事件类型确认 Worker 只产生一个 `workflow_dispatch`。同一分钟可能另有 `schedule` 运行。
2. 观察一次自愈 cron 的 Worker 日志。可通过创建一个测试用、超过 5 分钟仍为 active 的运行来验证取消；不要为测试取消正在需要的运行。确认 `409` 不会报错，其他逐项取消失败会在日志中一起出现，且 Worker 不会补跑。
3. 进行 Token 轮换或撤销时，先在 Cloudflare 更新 `GITHUB_TOKEN` Secret 并验证新 Token，再在 GitHub Token 页面撤销旧 Token。若怀疑泄露，立即撤销而不是等待过期，并检查 Actions/Worker 日志。
4. 不要在代码、提交、截图、日志或支持请求中泄露 PAT；定期审查 Token 的仓库范围与过期时间。Cloudflare 的免费额度和安全规则以官方当前规则为准，同时参阅 [Cron Triggers](https://developers.cloudflare.com/workers/configuration/cron-triggers/)、[Secrets](https://developers.cloudflare.com/workers/configuration/secrets/)、[Workers 价格](https://developers.cloudflare.com/workers/platform/pricing/) 以及 [GitHub fine-grained PAT](https://docs.github.com/en/authentication/keeping-your-account-and-data-secure/managing-your-personal-access-tokens)。

## 免费额度与费用提醒

- GitHub 当前说明：公开仓库使用 standard GitHub-hosted runners 免费；更大型 runner、私有仓库和其他收费资源的规则不同。启用 Fork 中的工作流前请查看最新的 [GitHub Actions billing and usage](https://docs.github.com/en/actions/concepts/billing-and-usage)，不要把“免费”理解成永久承诺。
- [Server酱官方文档（2026-07-17 更新）](https://sct.ftqq.com/docs/getting-started/sendkey/)写明 Server酱 Turbo 免费会员最多 5 条/天；Server酱³ 当前测试/免费状态及正式收费安排可能变化。部署前和运行期间都应以官方页面的最新额度、通道和收费说明为准。
- 工作流每天有 96 次检查，但只对尚未成功推送的在售商品发送聚合通知。失败后的重试或未来规则变化仍可能影响 Server酱额度，请在 Actions 和 Server酱控制台观察实际使用量。

## API 响应

`/api/products` 与静态 `products.json` 返回完整结果；`/api/onsale` 与静态 `onsale.json` 仅返回当前在售商品。响应的主要结构如下：

```json
{
  "code": 200,
  "message": "success",
  "data": {
    "time_slots": [{"label": "08:00-12:00"}],
    "products": [],
    "on_sale_count": 0,
    "total_count": 0,
    "updated_at": "2026-09-10 08:02:00"
  }
}
```

## 安全说明

- `config.json`、`public/` 和 `pages/` 均不会作为源码提交；公开仓库只保留空凭证的 `config.example.json`。
- `gh-pages/state.json` 是公开文件，只保存日期、时段和商品名组成的去重标识，不保存 SendKey。
- Actions 只把 `SERVERCHAN_SENDKEY` 注入运行进程。错误信息和静态文件不应包含 SendKey 或完整私有推送 URL。

## 来源与参考说明

本项目最初基于上游 [kanzakine/Roco-API](https://github.com/kanzakine/Roco-API) 改造。无服务器改造另外只参考了 `ALLCAPS-Droid/roco-merchant-notifier` 的 GitHub Actions 单次运行思路，未复制该参考项目的代码或素材。

## 许可证

上游项目未提供明确的许可证文件；使用或再分发前请自行确认授权范围。
