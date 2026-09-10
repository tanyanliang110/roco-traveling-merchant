package main

// ShopSlot 时段信息。
type ShopSlot struct {
	Label string `json:"label"`
}

// Product 商品。
type Product struct {
	Name       string `json:"name"`
	Price      string `json:"price"`
	Limit      string `json:"limit"`
	Category   string `json:"category"`
	Desc       string `json:"desc"`
	ImageURL   string `json:"image_url"`
	IsOnSale   bool   `json:"is_on_sale"`
	HasEnded   bool   `json:"has_ended"`
	IsUpcoming bool   `json:"is_upcoming"`
	RemainStr  string `json:"remain"`
	SlotLabel  string `json:"slot_label"`
	StartAt    int64  `json:"start_at"`
	EndAt      int64  `json:"end_at"`
}

// CrawlResult 爬取结果（整个缓存）。
type CrawlResult struct {
	TimeSlots   []ShopSlot `json:"time_slots"`
	Products    []Product  `json:"products"`
	OnSaleCount int        `json:"on_sale_count"`
	TotalCount  int        `json:"total_count"`
	UpdatedAt   string     `json:"updated_at"`
}

// APIResponse API 通用响应格式。
type APIResponse struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}
