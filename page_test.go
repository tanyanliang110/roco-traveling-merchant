package main

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func pageTestResult() CrawlResult {
	return CrawlResult{
		TimeSlots: []ShopSlot{
			{Label: "08:00-12:00"},
			{Label: "12:00-16:00"},
			{Label: "16:00-20:00"},
		},
		Products: []Product{
			{
				Name:      "在售商品",
				Price:     "100",
				Limit:     "限购1",
				Category:  "材料",
				IsOnSale:  true,
				RemainStr: "STATIC_REMAIN_SHOULD_NOT_APPEAR",
				SlotLabel: "12:00-16:00",
				StartAt:   1788936000,
				EndAt:     1788950400,
			},
			{
				Name:       "未开始商品",
				Price:      "200",
				Limit:      "限购2",
				Category:   "道具",
				IsUpcoming: true,
				SlotLabel:  "16:00-20:00",
				StartAt:    1788950400,
				EndAt:      1788964800,
			},
			{
				Name:      "已结束商品",
				Price:     "300",
				Limit:     "限购3",
				Category:  "其他",
				HasEnded:  true,
				SlotLabel: "08:00-12:00",
				StartAt:   1788921600,
				EndAt:     1788936000,
			},
		},
		OnSaleCount: 1,
		TotalCount:  3,
		UpdatedAt:   "2026-09-09 13:00:00",
	}
}

// This test fails if countdowns regress to generation-time remain strings or
// stop comparing the supplied Unix boundaries with the browser clock.
func TestRenderedPageContainsDynamicProductTimes(t *testing.T) {
	var out bytes.Buffer
	if err := RenderPage(&out, pageTestResult()); err != nil {
		t.Fatal(err)
	}
	body := out.String()
	for _, want := range []string{
		`data-start-at="1788936000"`,
		`data-end-at="1788950400"`,
		"setInterval",
		"Date.now()",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("page lacks dynamic countdown token %q", want)
		}
	}
	if strings.Contains(body, "STATIC_REMAIN_SHOULD_NOT_APPEAR") || strings.Contains(body, "data-remain") {
		t.Fatal("page still relies on the generation-time remain string")
	}
}

// This test fails if the static page stops refreshing from its sibling API
// envelope, local mode loses its API fallback, or fetched values can be
// interpreted as HTML instead of text.
func TestRenderedPageRefreshesFromSiblingJSONWithSafeLocalFallback(t *testing.T) {
	var out bytes.Buffer
	if err := RenderPage(&out, pageTestResult()); err != nil {
		t.Fatal(err)
	}
	body := out.String()
	staticFetch := "loadSnapshot('products.json')"
	localFetch := "loadSnapshot('/api/products')"
	staticAt, localAt := strings.Index(body, staticFetch), strings.Index(body, localFetch)
	if staticAt < 0 || localAt < 0 || staticAt > localAt {
		t.Fatalf("page must try sibling products.json before local API fallback: static=%d local=%d", staticAt, localAt)
	}
	for _, want := range []string{
		"if(!response.ok)",
		"payload.code!==200",
		"renderSnapshot",
		"document.createElement",
		"textContent=",
		"replaceChildren",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("page lacks safe JSON refresh contract %q", want)
		}
	}
	for _, unsafe := range []string{"innerHTML", "insertAdjacentHTML", "document.write"} {
		if strings.Contains(body, unsafe) {
			t.Fatalf("page refresh uses unsafe HTML sink %q", unsafe)
		}
	}
}

// This test fails if any crawler state is rendered with the wrong visual
// status, making upcoming and ended products indistinguishable.
func TestRenderedPageShowsThreeVisualStatuses(t *testing.T) {
	var out bytes.Buffer
	if err := RenderPage(&out, pageTestResult()); err != nil {
		t.Fatal(err)
	}
	body := out.String()
	for _, want := range []string{
		`class="card onsale"`,
		`class="card upcoming"`,
		`class="card ended"`,
		"✅ 在售",
		"⏳ 未开始",
		"❌ 已结束",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("page lacks status marker %q", want)
		}
	}
}

// This test fails if values originating in the crawled page are interpolated
// as trusted HTML instead of being escaped by html/template.
func TestRenderedPageEscapesUntrustedValues(t *testing.T) {
	result := pageTestResult()
	result.UpdatedAt = `<img src=x onerror="alert(1)">`
	result.TimeSlots[0].Label = `<script>alert("slot")</script>`
	result.Products[0].Name = `<script>alert("name")</script>`

	var out bytes.Buffer
	if err := RenderPage(&out, result); err != nil {
		t.Fatal(err)
	}
	body := out.String()
	for _, unsafe := range []string{
		`<img src=x onerror="alert(1)">`,
		`<script>alert("slot")</script>`,
		`<script>alert("name")</script>`,
	} {
		if strings.Contains(body, unsafe) {
			t.Fatalf("page contains unescaped value %q", unsafe)
		}
	}
	for _, escaped := range []string{"&lt;img", "&lt;script&gt;"} {
		if !strings.Contains(body, escaped) {
			t.Fatalf("page lacks escaped representation %q", escaped)
		}
	}
}

// This test fails if the local route grows a second, divergent page renderer.
func TestHomeHandlerUsesSharedPageRendering(t *testing.T) {
	result := pageTestResult()
	var want bytes.Buffer
	if err := RenderPage(&want, result); err != nil {
		t.Fatal(err)
	}

	recorder := httptest.NewRecorder()
	newServerWithResult(result).Handler().ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
	}
	if got := recorder.Body.String(); got != want.String() {
		t.Fatal("local home response differs from RenderPage output")
	}
}
