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
