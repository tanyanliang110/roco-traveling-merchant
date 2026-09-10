package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
)

// FetchProducts requests the merchant page and parses its products.
func FetchProducts(ctx context.Context, client *http.Client, targetURL string, now time.Time) (CrawlResult, error) {
	if client == nil {
		return CrawlResult{}, errors.New("http client is required")
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, targetURL, nil)
	if err != nil {
		return CrawlResult{}, fmt.Errorf("create merchant request: %w", err)
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	req.Header.Set("Accept-Language", "zh-CN,zh;q=0.9,en;q=0.8")

	resp, err := client.Do(req)
	if err != nil {
		return CrawlResult{}, fmt.Errorf("fetch merchant page: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return CrawlResult{}, fmt.Errorf("merchant response status: %s", resp.Status)
	}
	return ParseProducts(resp.Body, now)
}

// ParseProducts parses merchant HTML into a crawl result using now for state.
func ParseProducts(r io.Reader, now time.Time) (CrawlResult, error) {
	doc, err := goquery.NewDocumentFromReader(r)
	if err != nil {
		return CrawlResult{}, fmt.Errorf("parse merchant HTML: %w", err)
	}

	slots := make([]ShopSlot, 0)
	for _, label := range extractTimeSlots(doc) {
		slots = append(slots, ShopSlot{Label: label})
	}
	if len(slots) == 0 {
		slots = []ShopSlot{{Label: "08:00-12:00"}, {Label: "12:00-16:00"}, {Label: "16:00-20:00"}, {Label: "20:00-24:00"}}
	}

	var products []Product
	doc.Find(".all_show").Each(func(_ int, s *goquery.Selection) {
		name := strings.TrimSpace(s.Find(".shop_name").Text())
		if name == "" {
			return
		}
		price := strings.TrimSpace(strings.Replace(strings.TrimSpace(s.Find(".shop_price").Text()), "价格：", "", 1))
		limit := strings.TrimSpace(s.Find(".gitem em").First().Text())
		imageURL, _ := s.Find(".gitem img").Attr("src")
		onclick, _ := s.Attr("onclick")
		category, desc := parseOnclick(onclick)

		product := Product{Name: name, Price: price, Limit: limit, Category: category, Desc: desc, ImageURL: imageURL}
		dataTime, _ := s.Attr("data-time")
		if endAt, err := strconv.ParseInt(dataTime, 10, 64); err == nil {
			endTime := time.Unix(endAt, 0)
			startTime := endTime.Add(-4 * time.Hour)
			product.StartAt = startTime.Unix()
			product.EndAt = endTime.Unix()
			product.SlotLabel = slotForTime(slots, startTime, endTime)
			switch {
			case now.Before(startTime):
				product.IsUpcoming = true
			case now.Before(endTime):
				product.IsOnSale = true
				diff := endTime.Sub(now)
				product.RemainStr = fmt.Sprintf("%02d:%02d:%02d", int(diff.Hours()), int(diff.Minutes())%60, int(diff.Seconds())%60)
			default:
				product.HasEnded = true
			}
		}
		products = append(products, product)
	})

	if len(products) == 0 {
		return CrawlResult{}, errors.New("merchant page contains no products")
	}
	onSaleCount := 0
	for _, product := range products {
		if product.IsOnSale {
			onSaleCount++
		}
	}
	return CrawlResult{TimeSlots: slots, Products: products, OnSaleCount: onSaleCount, TotalCount: len(products), UpdatedAt: now.In(beijingLocation).Format("2006-01-02 15:04:05")}, nil
}

func extractTimeSlots(doc *goquery.Document) []string {
	var slots []string
	for _, selector := range []string{"li[class*='check_']", ".sp-time-con li", ".time-con li"} {
		slots = nil
		doc.Find(selector).Each(func(_ int, s *goquery.Selection) {
			text := strings.TrimSpace(s.Text())
			if text == "" {
				return
			}
			var parts []string
			s.Find("em").Each(func(_ int, em *goquery.Selection) { parts = append(parts, strings.TrimSpace(em.Text())) })
			if len(parts) == 2 {
				slots = append(slots, parts[0]+"-"+parts[1])
			} else {
				slots = append(slots, text)
			}
		})
		if len(slots) > 0 {
			break
		}
	}
	return slots
}

func currentSlot(slots []ShopSlot, now time.Time) string {
	if len(slots) == 0 {
		return "未知时段"
	}
	currentHour := now.In(beijingLocation).Hour()
	for _, slot := range slots {
		startHour, endHour := parseSlotHours(slot.Label)
		if currentHour >= startHour && currentHour < endHour {
			return slot.Label
		}
	}
	return slots[len(slots)-1].Label
}

func slotForTime(slots []ShopSlot, startTime, _ time.Time) string {
	if len(slots) == 0 {
		return "未知时段"
	}
	startHour := startTime.In(beijingLocation).Hour()
	for _, slot := range slots {
		slotStartHour, slotEndHour := parseSlotHours(slot.Label)
		if startHour >= slotStartHour && startHour < slotEndHour {
			return slot.Label
		}
	}
	return slots[len(slots)-1].Label
}

func parseSlotHours(label string) (int, int) {
	parts := strings.Split(label, "-")
	if len(parts) != 2 {
		return 0, 24
	}
	startHour, _ := strconv.Atoi(strings.Split(parts[0], ":")[0])
	endHour, _ := strconv.Atoi(strings.Split(parts[1], ":")[0])
	return startHour, endHour
}

func parseOnclick(onclick string) (category, desc string) {
	re := regexp.MustCompile(`'([^']*)','([^']*)','([^']*)','([^']*)'`)
	matches := re.FindStringSubmatch(onclick)
	if len(matches) >= 5 {
		return matches[3], matches[4]
	}
	return "", ""
}
