package main

import (
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

// ============================================================
// 推送追踪 — 控制推送时机和次数
// ============================================================

// pushRecord 保存已推送过的商品标识
type pushRecord struct {
	Date      string            // 日期 YYYY-MM-DD
	SentNames map[string]bool   // 已推送过的商品名
	PastNames map[string]string // 过往商品名→时段（今日已结束的）
}

var (
	pushTracker     pushRecord
	pushTrackerLock sync.Mutex
)

// initPushTracker 初始化或重置推送追踪
func initPushTracker() {
	today := time.Now().Format("2006-01-02")
	pushTracker = pushRecord{
		Date:      today,
		SentNames: make(map[string]bool),
		PastNames: make(map[string]string),
	}
}

// needPush 判断是否需要推送：有新上架商品时推送
func needPush(onsaleProducts []Product) bool {
	pushTrackerLock.Lock()
	defer pushTrackerLock.Unlock()

	// 如果日期变了，重置追踪
	today := time.Now().Format("2006-01-02")
	if pushTracker.Date != today {
		// 跨天时：保留当前在售商品的已推送标记，避免00:00误推送
		oldSent := pushTracker.SentNames
		initPushTracker()
		// 将旧日期的在售商品标记为已推送
		for _, p := range onsaleProducts {
			if oldSent[p.Name] {
				pushTracker.SentNames[p.Name] = true
			}
		}
	}

	// 检查是否有新商品未推送过
	for _, p := range onsaleProducts {
		if !pushTracker.SentNames[p.Name] {
			return true
		}
	}
	return false
}

// markPushed 标记商品已推送
func markPushed(onsaleProducts []Product) {
	pushTrackerLock.Lock()
	defer pushTrackerLock.Unlock()

	for _, p := range onsaleProducts {
		pushTracker.SentNames[p.Name] = true
	}
}

// updatePastProducts 更新今日过往（已结束）商品
// 重要：只记录属于今天（CST时区）时段的已结束商品，
// 避免跨天时把前一天最后时段（20:00-24:00）的商品记入新的一天。
func updatePastProducts(products []Product) {
	pushTrackerLock.Lock()
	defer pushTrackerLock.Unlock()

	today := time.Now().Format("2006-01-02")
	if pushTracker.Date != today {
		initPushTracker()
	}

	cstZone := time.FixedZone("CST", 8*3600)
	now := time.Now().In(cstZone)

	for _, p := range products {
		if !p.HasEnded || p.SlotLabel == "" {
			continue
		}

		// 解析时段标签，如 "08:00-12:00" → 开始小时=8
		parts := strings.Split(p.SlotLabel, "-")
		if len(parts) != 2 {
			continue
		}
		startHour, err := strconv.Atoi(strings.Split(parts[0], ":")[0])
		if err != nil {
			continue
		}

		// 判断该商品时段是否属于今天：
		// 如果当前小时 < 时段开始小时，说明这是前一天的时段
		// 例如：在 00:05 看到 "20:00-24:00" → 0 < 20 → 属于前一天，跳过
		if now.Hour() < startHour {
			continue
		}

		pushTracker.PastNames[p.Name] = p.SlotLabel
	}
}

// getPastProducts 获取今日过往商品（简短格式）
func getPastProducts() []string {
	pushTrackerLock.Lock()
	defer pushTrackerLock.Unlock()

	if pushTracker.Date != time.Now().Format("2006-01-02") {
		return nil
	}

	var result []string
	for name, slot := range pushTracker.PastNames {
		result = append(result, fmt.Sprintf("- %s（%s）", name, slot))
	}
	return result
}

// isFirstBatch 判断是否是今日首批推送（过往商品为空则为首批）
func isFirstBatch() bool {
	pushTrackerLock.Lock()
	defer pushTrackerLock.Unlock()
	return len(pushTracker.PastNames) == 0
}

// ============================================================
// 主函数
// ============================================================
func main() {
	if err := run(os.Args[1:], os.Getenv); err != nil {
		log.Fatalf("[错误] %v", err)
	}
}

// buildPushMessage 构造推送内容
// 格式：时段 → 在售商品详情 → 今日过往商品（非首批时显示）
func buildPushMessage(onsaleProducts []Product, currentSlotLabel string) string {
	return buildPushMessageWithPageLink(onsaleProducts, currentSlotLabel, "http://localhost"+appConfig.Server.Port)
}

// buildOncePushMessage omits a page link because once mode has no configured
// canonical GitHub Pages URL.
func buildOncePushMessage(onsaleProducts []Product, currentSlotLabel string) string {
	return buildPushMessageWithPageLink(onsaleProducts, currentSlotLabel, "")
}

func buildPushMessageWithPageLink(onsaleProducts []Product, currentSlotLabel, pageLink string) string {
	var b strings.Builder
	onsaleProducts = prepareNotificationProducts(onsaleProducts, nil)

	// 时段标题
	b.WriteString(fmt.Sprintf("## 🕐 %s\n\n", currentSlotLabel))

	// 在售商品
	for _, p := range onsaleProducts {
		if isImportantOnSale(p) {
			b.WriteString("❗ ")
		}
		b.WriteString(fmt.Sprintf("**%s**  💰 %s", p.Name, p.Price))
		if p.Limit != "" {
			b.WriteString(fmt.Sprintf("  📦 %s", p.Limit))
		}
		if p.Category != "" {
			b.WriteString(fmt.Sprintf("  📂 %s", p.Category))
		}
		if p.RemainStr != "" {
			b.WriteString(fmt.Sprintf("  ⏱ %s", p.RemainStr))
		}
		b.WriteString("\n\n")
	}

	// 今日过往商品（只在有记录时显示）
	if !isFirstBatch() {
		pastList := getPastProducts()
		if len(pastList) > 0 {
			b.WriteString("---\n")
			b.WriteString("### 📜 今日过往商品\n\n")
			for _, item := range pastList {
				b.WriteString(item + "\n")
			}
			b.WriteString("\n")
		}
	}

	if pageLink != "" {
		b.WriteString("---\n")
		b.WriteString("📡 [查看完整页面](" + pageLink + ")\n")
	}
	return b.String()
}
