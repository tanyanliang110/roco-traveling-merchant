package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"
)

// Server owns the local crawl cache and HTTP API boundary.
type Server struct {
	cache  CrawlResult
	mu     sync.RWMutex
	client *http.Client
	config Config
	now    func() time.Time
}

// NewServer creates the local server with production defaults.
func NewServer(config Config) *Server {
	return &Server{
		client: &http.Client{Timeout: 15 * time.Second},
		config: config,
		now:    time.Now,
	}
}

// Refresh fetches a complete result before atomically replacing the cache.
func (s *Server) Refresh(ctx context.Context) error {
	client := s.client
	if client == nil {
		client = &http.Client{Timeout: 15 * time.Second}
	}
	clock := s.now
	if clock == nil {
		clock = time.Now
	}
	now := clock()

	result, err := FetchProducts(ctx, client, s.config.Crawl.TargetURL, now)
	if err != nil {
		return err
	}

	updatePastProducts(result.Products)
	s.mu.Lock()
	s.cache = result
	s.mu.Unlock()

	fmt.Printf("[完成] 爬取完成: %d 件商品, %d 件在售中\n", result.TotalCount, result.OnSaleCount)
	s.notify(result, now)
	return nil
}

func (s *Server) notify(result CrawlResult, now time.Time) {
	if !ServerChanEnabled() || result.OnSaleCount == 0 {
		return
	}

	var onsaleProducts []Product
	for _, product := range result.Products {
		if product.IsOnSale {
			onsaleProducts = append(onsaleProducts, product)
		}
	}
	if !needPush(onsaleProducts) {
		return
	}

	title := fmt.Sprintf("🏪 远行商人更新 · %d 件在售", result.OnSaleCount)
	SendServerChan(title, buildPushMessage(onsaleProducts, currentSlot(result.TimeSlots, now)))
	markPushed(onsaleProducts)
}

// Handler returns an isolated router for the existing local API routes.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/", s.handleHome)
	mux.HandleFunc("/api/products", s.handleProducts)
	mux.HandleFunc("/api/onsale", s.handleOnSale)
	return mux
}

// Run starts the local HTTP API and periodically refreshes its cache.
func (s *Server) Run(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if s.config.Server.Port == "" {
		s.config.Server.Port = ":8008"
	}
	if s.config.Crawl.Interval <= 0 {
		s.config.Crawl.Interval = 3
	}

	fmt.Println("[任务] 首次爬取远行商人数据...")
	if err := s.Refresh(ctx); err != nil {
		log.Printf("[错误] 首次爬取失败，服务将使用空缓存启动: %v", err)
	}

	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	ticker := time.NewTicker(time.Duration(s.config.Crawl.Interval) * time.Minute)
	defer ticker.Stop()
	go s.refreshLoop(runCtx, ticker)

	server := &http.Server{Addr: s.config.Server.Port, Handler: s.Handler()}
	stopped := make(chan struct{})
	defer close(stopped)
	go func() {
		select {
		case <-runCtx.Done():
			shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer shutdownCancel()
			if err := server.Shutdown(shutdownCtx); err != nil {
				log.Printf("[错误] 服务器关闭失败: %v", err)
			}
		case <-stopped:
		}
	}()

	fmt.Println("\n========================================")
	fmt.Printf("[启动] API 服务已启动: http://localhost%s\n", s.config.Server.Port)
	fmt.Println("   GET /api/products  - 全部商品")
	fmt.Println("   GET /api/onsale    - 在售商品")
	fmt.Println("========================================")
	fmt.Printf("[定时] 每 %d 分钟自动爬取更新数据\n", s.config.Crawl.Interval)

	err := server.ListenAndServe()
	if errors.Is(err, http.ErrServerClosed) && ctx.Err() != nil {
		return nil
	}
	return err
}

func (s *Server) refreshLoop(ctx context.Context, ticker *time.Ticker) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			fmt.Println("[定时] 爬取中...")
			if err := s.Refresh(ctx); err != nil {
				log.Printf("[错误] 爬取失败: %v", err)
			}
		}
	}
}

func (s *Server) snapshot() CrawlResult {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.cache
}

// sortProducts sorts products by availability, slot, then name.
func sortProducts(products []Product, slots []ShopSlot) {
	slotOrder := make(map[string]int)
	for i, slot := range slots {
		slotOrder[slot.Label] = i
	}

	for i := 0; i < len(products); i++ {
		for j := i + 1; j < len(products); j++ {
			if products[i].IsOnSale != products[j].IsOnSale {
				if !products[i].IsOnSale {
					products[i], products[j] = products[j], products[i]
				}
				continue
			}
			iOrder := slotOrder[products[i].SlotLabel]
			jOrder := slotOrder[products[j].SlotLabel]
			if iOrder != jOrder {
				if iOrder > jOrder {
					products[i], products[j] = products[j], products[i]
				}
				continue
			}
			if products[i].Name > products[j].Name {
				products[i], products[j] = products[j], products[i]
			}
		}
	}
}

func (s *Server) handleHome(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := RenderPage(w, s.snapshot()); err != nil {
		log.Printf("[错误] 首页渲染失败: %v", err)
	}
}

func (s *Server) handleProducts(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(productsAPIResponse(s.snapshot()))
}

func (s *Server) handleOnSale(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(onSaleAPIResponse(s.snapshot()))
}
