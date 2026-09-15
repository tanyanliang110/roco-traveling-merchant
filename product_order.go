package main

import (
	"fmt"
	"sort"
)

// importantProductNames is the single source of truth for products that need
// extra attention while they are on sale. Its order is also their priority.
var importantProductNames = []string{
	"炫彩精灵蛋",
	"奇异血脉秘药",
	"首领血脉秘药",
	"棱镜球",
}

func importantProductRank(name string) int {
	for rank, importantName := range importantProductNames {
		if name == importantName {
			return rank
		}
	}
	return -1
}

func isImportantOnSale(product Product) bool {
	return product.IsOnSale && importantProductRank(product.Name) >= 0
}

func sortedProducts(products []Product, slots []ShopSlot) []Product {
	sorted := append([]Product(nil), products...)
	sortProducts(sorted, slots)
	return sorted
}

// sortProducts orders important on-sale products first, then other on-sale,
// upcoming, and ended products. Important products keep ordinary status order
// when they are not currently on sale.
func sortProducts(products []Product, slots []ShopSlot) {
	slotOrder := make(map[string]int, len(slots))
	for i, slot := range slots {
		slotOrder[slot.Label] = i
	}

	sort.SliceStable(products, func(i, j int) bool {
		left, right := products[i], products[j]
		leftGroup, rightGroup := productOrderGroup(left), productOrderGroup(right)
		if leftGroup != rightGroup {
			return leftGroup < rightGroup
		}
		if leftGroup == 0 {
			leftRank, rightRank := importantProductRank(left.Name), importantProductRank(right.Name)
			if leftRank != rightRank {
				return leftRank < rightRank
			}
		}
		leftSlot, rightSlot := productSlotOrder(left.SlotLabel, slotOrder, len(slots)), productSlotOrder(right.SlotLabel, slotOrder, len(slots))
		if leftSlot != rightSlot {
			return leftSlot < rightSlot
		}
		return left.Name < right.Name
	})
}

func productOrderGroup(product Product) int {
	switch {
	case isImportantOnSale(product):
		return 0
	case product.IsOnSale:
		return 1
	case product.IsUpcoming:
		return 2
	default:
		return 3
	}
}

func productSlotOrder(label string, order map[string]int, fallback int) int {
	if rank, ok := order[label]; ok {
		return rank
	}
	return fallback
}

func prepareNotificationProducts(products []Product, slots []ShopSlot) []Product {
	onSale := make([]Product, 0, len(products))
	for _, product := range products {
		if product.IsOnSale {
			onSale = append(onSale, product)
		}
	}
	sortProducts(onSale, slots)
	return onSale
}

func notificationTitle(products []Product) string {
	onSaleCount := 0
	hasImportant := false
	for _, product := range products {
		if !product.IsOnSale {
			continue
		}
		onSaleCount++
		hasImportant = hasImportant || isImportantOnSale(product)
	}
	prefix := ""
	if hasImportant {
		prefix = "❗ "
	}
	return fmt.Sprintf("%s🏪 远行商人更新 · %d 件在售", prefix, onSaleCount)
}
