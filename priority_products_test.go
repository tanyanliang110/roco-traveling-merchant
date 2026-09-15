package main

import (
	"bytes"
	"context"
	"reflect"
	"strings"
	"testing"
	"time"
)

func priorityTestResult() CrawlResult {
	return CrawlResult{
		TimeSlots: []ShopSlot{
			{Label: "08:00-12:00"},
			{Label: "12:00-16:00"},
			{Label: "16:00-20:00"},
		},
		Products: []Product{
			{Name: "已结束普通商品", HasEnded: true, SlotLabel: "08:00-12:00"},
			{Name: "普通在售商品", IsOnSale: true, SlotLabel: "12:00-16:00"},
			{Name: "棱镜球", IsOnSale: true, SlotLabel: "12:00-16:00"},
			{Name: "首领血脉秘药", IsOnSale: true, SlotLabel: "12:00-16:00"},
			{Name: "炫彩精灵蛋", IsUpcoming: true, SlotLabel: "16:00-20:00"},
			{Name: "即将开售普通商品", IsUpcoming: true, SlotLabel: "16:00-20:00"},
			{Name: "奇异血脉秘药", IsOnSale: true, SlotLabel: "12:00-16:00"},
			{Name: "炫彩精灵蛋", IsOnSale: true, SlotLabel: "12:00-16:00"},
		},
		OnSaleCount: 5,
		TotalCount:  8,
		UpdatedAt:   "2026-09-15 13:23:12",
	}
}

func productNames(products []Product) []string {
	names := make([]string, len(products))
	for i, product := range products {
		names[i] = product.Name
	}
	return names
}

func TestSortProductsPrioritizesOnlyOnSaleImportantProductsAndEndsLast(t *testing.T) {
	result := priorityTestResult()
	sortProducts(result.Products, result.TimeSlots)

	want := []string{
		"炫彩精灵蛋",
		"奇异血脉秘药",
		"首领血脉秘药",
		"棱镜球",
		"普通在售商品",
		"即将开售普通商品",
		"炫彩精灵蛋",
		"已结束普通商品",
	}
	if got := productNames(result.Products); !reflect.DeepEqual(got, want) {
		t.Fatalf("sorted names = %v, want %v", got, want)
	}
}

func TestProductsAPIResponseSortsACopyWithoutChangingNamesOrInput(t *testing.T) {
	result := priorityTestResult()
	original := append([]Product(nil), result.Products...)

	response := productsAPIResponse(result)
	data, ok := response.Data.(CrawlResult)
	if !ok {
		t.Fatalf("response data type = %T, want CrawlResult", response.Data)
	}
	wantFirst := []string{"炫彩精灵蛋", "奇异血脉秘药", "首领血脉秘药", "棱镜球"}
	if got := productNames(data.Products[:4]); !reflect.DeepEqual(got, wantFirst) {
		t.Fatalf("first API products = %v, want %v", got, wantFirst)
	}
	if data.Products[len(data.Products)-1].Name != "已结束普通商品" {
		t.Fatalf("last API product = %q, want ended product", data.Products[len(data.Products)-1].Name)
	}
	if !reflect.DeepEqual(result.Products, original) {
		t.Fatal("productsAPIResponse mutated the crawl result")
	}
	for _, product := range data.Products {
		if strings.HasPrefix(product.Name, "❗") {
			t.Fatalf("API product name was decorated: %q", product.Name)
		}
	}
}

func TestRenderedPageKeepsSaleOrderAfterSnapshotRefreshAndMarksOnlySalePriority(t *testing.T) {
	var out bytes.Buffer
	if err := RenderPage(&out, priorityTestResult()); err != nil {
		t.Fatal(err)
	}
	body := out.String()

	importantAt := strings.Index(body, "❗ 炫彩精灵蛋")
	regularAt := strings.Index(body, ">普通在售商品</strong>")
	upcomingAt := strings.Index(body, ">即将开售普通商品</strong>")
	endedAt := strings.Index(body, ">已结束普通商品</strong>")
	if importantAt < 0 || regularAt < 0 || upcomingAt < 0 || endedAt < 0 || !(importantAt < regularAt && regularAt < upcomingAt && upcomingAt < endedAt) {
		t.Fatalf("rendered product order is wrong: important=%d regular=%d upcoming=%d ended=%d", importantAt, regularAt, upcomingAt, endedAt)
	}
	if strings.Count(body, "❗ 炫彩精灵蛋") != 1 {
		t.Fatalf("important marker count for mixed-status 炫彩精灵蛋 = %d, want 1", strings.Count(body, "❗ 炫彩精灵蛋"))
	}
	for _, want := range []string{"sortSnapshotProducts", "priorityProductRank", "sortProductCards"} {
		if !strings.Contains(body, want) {
			t.Fatalf("page lacks client-side ordering behavior %q", want)
		}
	}
	const prioritySource = `var priorityProductNames=["炫彩精灵蛋","奇异血脉秘药","首领血脉秘药","棱镜球"];`
	if strings.Count(body, prioritySource) != 1 {
		t.Fatalf("page must inject the Go priority list once, count = %d", strings.Count(body, prioritySource))
	}
	if !strings.Contains(body, "sortSnapshotProducts(snapshot.products,snapshot.time_slots).forEach") {
		t.Fatal("renderSnapshot does not consume the sorted snapshot")
	}
	if !strings.Contains(body, "document.querySelectorAll('[data-start-at][data-end-at]').forEach(function(card){updateCard(card,now)});\n  sortProductCards();") {
		t.Fatal("countdown refresh does not reorder cards after status transitions")
	}
}

func TestRenderedPageUsesDeterministicCodePointNameOrdering(t *testing.T) {
	var out bytes.Buffer
	if err := RenderPage(&out, priorityTestResult()); err != nil {
		t.Fatal(err)
	}
	body := out.String()
	if strings.Contains(body, "localeCompare") {
		t.Fatal("browser locale collation can disagree with Go product ordering")
	}
	for _, want := range []string{
		"function compareProductNames(left,right)",
		"Array.from(stringValue(left))",
		"codePointAt(0)",
		"return compareProductNames(left.name,right.name)",
		"return compareProductNames(left.getAttribute('data-product-name'),right.getAttribute('data-product-name'))",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("page lacks deterministic name ordering contract %q", want)
		}
	}
}

func TestPushMessagePrioritizesAndMarksOnlyOnSaleImportantProducts(t *testing.T) {
	products := []Product{
		{Name: "普通在售商品", IsOnSale: true},
		{Name: "棱镜球", IsOnSale: true},
		{Name: "炫彩精灵蛋", IsUpcoming: true},
		{Name: "奇异血脉秘药", IsOnSale: true},
	}
	notificationProducts := prepareNotificationProducts(products, nil)
	message := buildOncePushMessage(notificationProducts, "12:00-16:00")

	oddAt := strings.Index(message, "❗ **奇异血脉秘药**")
	prismAt := strings.Index(message, "❗ **棱镜球**")
	regularAt := strings.Index(message, "**普通在售商品**")
	if oddAt < 0 || prismAt < 0 || regularAt < 0 || !(oddAt < prismAt && prismAt < regularAt) {
		t.Fatalf("notification order or markers are wrong:\n%s", message)
	}
	if strings.Contains(message, "❗ **炫彩精灵蛋**") {
		t.Fatalf("upcoming important product was marked:\n%s", message)
	}
	if strings.Contains(message, "**炫彩精灵蛋**") {
		t.Fatalf("upcoming product entered the notification:\n%s", message)
	}
}

func TestNotificationTitleMarksOnlySalePriority(t *testing.T) {
	withPriority := []Product{
		{Name: "普通商品", IsOnSale: true},
		{Name: "棱镜球", IsOnSale: true},
	}
	if got := notificationTitle(withPriority); got != "❗ 🏪 远行商人更新 · 2 件在售" {
		t.Fatalf("priority title = %q", got)
	}

	withoutSalePriority := []Product{
		{Name: "普通商品", IsOnSale: true},
		{Name: "棱镜球", IsUpcoming: true},
	}
	if got := notificationTitle(withoutSalePriority); got != "🏪 远行商人更新 · 1 件在售" {
		t.Fatalf("ordinary title = %q", got)
	}
}

func TestPriorityDecorationDoesNotChangeNotificationKey(t *testing.T) {
	now := beijingDate(2026, time.September, 15, 13, 23, 12)
	product := Product{Name: "炫彩精灵蛋", IsOnSale: true, SlotLabel: "12:00-16:00"}
	if got := NotificationKey(product, now); got != "2026-09-15 | 12:00-16:00 | 炫彩精灵蛋" {
		t.Fatalf("notification key = %q", got)
	}
}

func TestRunOncePassesOnlySortedOnSaleProductsToNotifier(t *testing.T) {
	dir := t.TempDir()
	cfg := testConfig()
	cfg.ServerChan.SendKey = "SCTtest"
	var notified []Product
	err := RunOnce(context.Background(), cfg, dir, dir+"/state.json", OnceDependencies{
		Fetch: func(context.Context) (CrawlResult, error) {
			return priorityTestResult(), nil
		},
		Notify: func(_ context.Context, products []Product) error {
			notified = append([]Product(nil), products...)
			return nil
		},
		Now: func() time.Time {
			return beijingDate(2026, time.September, 15, 13, 23, 12)
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"炫彩精灵蛋", "奇异血脉秘药", "首领血脉秘药", "棱镜球", "普通在售商品"}
	if got := productNames(notified); !reflect.DeepEqual(got, want) {
		t.Fatalf("notified names = %v, want %v", got, want)
	}
}
