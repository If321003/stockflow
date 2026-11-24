package store

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"stockflow/backend/internal/model"

	"github.com/jackc/pgx/v5/pgxpool"
)

func setupTestStore(t *testing.T) (*Store, *pgxpool.Pool) {
	t.Helper()

	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		dsn = "postgres://stockflow:stockflow_pass@localhost:5432/stockflow?sslmode=disable"
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("connect db: %v", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		t.Fatalf("ping db: %v", err)
	}

	t.Cleanup(func() {
		pool.Close()
	})

	return New(pool), pool
}

func uniqueSKU(prefix string) string {
	return fmt.Sprintf("%s-%d", prefix, time.Now().UnixNano())
}

func cleanupProducts(t *testing.T, pool *pgxpool.Pool, productIDs []int, skus []string) {
	t.Helper()
	ctx := context.Background()

	if len(skus) > 0 {
		rows, err := pool.Query(ctx, `SELECT DISTINCT order_id FROM sales_order_items WHERE sku = ANY($1)`, skus)
		if err == nil {
			var orderIDs []int
			for rows.Next() {
				var oid int
				if scanErr := rows.Scan(&oid); scanErr == nil {
					orderIDs = append(orderIDs, oid)
				}
			}
			rows.Close()

			if len(orderIDs) > 0 {
				_, _ = pool.Exec(ctx, `DELETE FROM sales_order_items WHERE order_id = ANY($1)`, orderIDs)
				_, _ = pool.Exec(ctx, `DELETE FROM sales_orders WHERE id = ANY($1)`, orderIDs)
			}
		}
	}

	if len(productIDs) > 0 {
		_, _ = pool.Exec(ctx, `DELETE FROM stock_movements WHERE product_id = ANY($1)`, productIDs)
		_, _ = pool.Exec(ctx, `DELETE FROM products WHERE id = ANY($1)`, productIDs)
	}
}

func getStock(t *testing.T, pool *pgxpool.Pool, productID int) int {
	t.Helper()
	var stock int
	if err := pool.QueryRow(context.Background(), `SELECT stock FROM products WHERE id = $1`, productID).Scan(&stock); err != nil {
		t.Fatalf("query stock: %v", err)
	}
	return stock
}

func TestAdjustStockAndDeleteProtection(t *testing.T) {
	s, pool := setupTestStore(t)
	ctx := context.Background()

	p1, err := s.CreateProduct(ctx, model.CreateProductInput{
		SKU:   uniqueSKU("TST-ADJ"),
		Name:  "Test Adjust Product",
		Price: 50000,
		Stock: 10,
	})
	if err != nil {
		t.Fatalf("create p1: %v", err)
	}

	p2, err := s.CreateProduct(ctx, model.CreateProductInput{
		SKU:   uniqueSKU("TST-DEL"),
		Name:  "Test Deletable Product",
		Price: 25000,
		Stock: 5,
	})
	if err != nil {
		t.Fatalf("create p2: %v", err)
	}

	t.Cleanup(func() {
		cleanupProducts(t, pool, []int{p1.ID, p2.ID}, []string{p1.SKU, p2.SKU})
	})

	updated, mv, err := s.AdjustStock(ctx, p1.ID, model.AdjustStockInput{
		Type: "out",
		Qty:  4,
		Note: "manual test out",
	})
	if err != nil {
		t.Fatalf("adjust stock out: %v", err)
	}
	if updated.Stock != 6 || mv.Qty != 4 || mv.Type != "out" {
		t.Fatalf("unexpected result: stock=%d mv=%+v", updated.Stock, mv)
	}

	_, _, err = s.AdjustStock(ctx, p1.ID, model.AdjustStockInput{
		Type: "out",
		Qty:  7,
		Note: "should fail",
	})
	var stockErr *InsufficientStockError
	if !errors.As(err, &stockErr) {
		t.Fatalf("expected InsufficientStockError, got %v", err)
	}
	if stockErr.Available != 6 {
		t.Fatalf("expected available=6, got %d", stockErr.Available)
	}

	if err := s.DeleteProduct(ctx, p1.ID); !errors.Is(err, ErrProductHasHistory) {
		t.Fatalf("expected ErrProductHasHistory, got %v", err)
	}

	if err := s.DeleteProduct(ctx, p2.ID); err != nil {
		t.Fatalf("expected p2 delete to succeed, got %v", err)
	}
}

func TestCreateOrderAtomicRollback(t *testing.T) {
	s, pool := setupTestStore(t)
	ctx := context.Background()

	pA, err := s.CreateProduct(ctx, model.CreateProductInput{
		SKU:   uniqueSKU("TST-ATMA"),
		Name:  "Atomic Item A",
		Price: 10000,
		Stock: 10,
	})
	if err != nil {
		t.Fatalf("create pA: %v", err)
	}

	pB, err := s.CreateProduct(ctx, model.CreateProductInput{
		SKU:   uniqueSKU("TST-ATMB"),
		Name:  "Atomic Item B",
		Price: 20000,
		Stock: 2,
	})
	if err != nil {
		t.Fatalf("create pB: %v", err)
	}

	t.Cleanup(func() {
		cleanupProducts(t, pool, []int{pA.ID, pB.ID}, []string{pA.SKU, pB.SKU})
	})

	_, err = s.CreateOrder(ctx, model.CreateOrderInput{
		Customer: "Test Rollback",
		Items: []model.OrderItemInput{
			{SKU: pA.SKU, Qty: 3},
			{SKU: pB.SKU, Qty: 5},
		},
	})
	var stockErr *InsufficientStockError
	if !errors.As(err, &stockErr) {
		t.Fatalf("expected InsufficientStockError, got %v", err)
	}
	if stockErr.SKU != pB.SKU || stockErr.Available != 2 {
		t.Fatalf("unexpected stock error: %+v", stockErr)
	}

	if got := getStock(t, pool, pA.ID); got != 10 {
		t.Fatalf("pA stock should roll back to 10, got %d", got)
	}
	if got := getStock(t, pool, pB.ID); got != 2 {
		t.Fatalf("pB stock should stay 2, got %d", got)
	}
}

func TestCancelOrderRestoresStock(t *testing.T) {
	s, pool := setupTestStore(t)
	ctx := context.Background()

	p, err := s.CreateProduct(ctx, model.CreateProductInput{
		SKU:   uniqueSKU("TST-CNC"),
		Name:  "Cancel Order Product",
		Price: 50000,
		Stock: 10,
	})
	if err != nil {
		t.Fatalf("create product: %v", err)
	}

	t.Cleanup(func() {
		cleanupProducts(t, pool, []int{p.ID}, []string{p.SKU})
	})

	order, err := s.CreateOrder(ctx, model.CreateOrderInput{
		Customer: "Budi",
		Items:    []model.OrderItemInput{{SKU: p.SKU, Qty: 4}},
	})
	if err != nil {
		t.Fatalf("create order: %v", err)
	}
	if order.Total != 200000 || order.Status != "CREATED" {
		t.Fatalf("unexpected order: %+v", order)
	}
	if got := getStock(t, pool, p.ID); got != 6 {
		t.Fatalf("expected stock 6 after order, got %d", got)
	}

	cancelled, err := s.CancelOrder(ctx, order.ID)
	if err != nil {
		t.Fatalf("cancel order: %v", err)
	}
	if cancelled.Status != "CANCELLED" || cancelled.CancelledAt == nil {
		t.Fatalf("expected CANCELLED with timestamp, got %+v", cancelled)
	}
	if got := getStock(t, pool, p.ID); got != 10 {
		t.Fatalf("expected stock restored to 10, got %d", got)
	}

	if _, err := s.CancelOrder(ctx, order.ID); !errors.Is(err, ErrAlreadyCancelled) {
		t.Fatalf("expected ErrAlreadyCancelled on double cancel, got %v", err)
	}
	if got := getStock(t, pool, p.ID); got != 10 {
		t.Fatalf("double cancel must not change stock, got %d", got)
	}
}

func TestConcurrentOrdersPreventOversell(t *testing.T) {
	s, pool := setupTestStore(t)
	ctx := context.Background()

	p, err := s.CreateProduct(ctx, model.CreateProductInput{
		SKU:   uniqueSKU("TST-RACE"),
		Name:  "Concurrent Stock Product",
		Price: 10000,
		Stock: 5,
	})
	if err != nil {
		t.Fatalf("create product: %v", err)
	}

	t.Cleanup(func() {
		cleanupProducts(t, pool, []int{p.ID}, []string{p.SKU})
	})

	const workers = 10
	start := make(chan struct{})
	results := make(chan error, workers)

	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			<-start
			_, orderErr := s.CreateOrder(ctx, model.CreateOrderInput{
				Customer: fmt.Sprintf("Buyer-%d", idx),
				Items:    []model.OrderItemInput{{SKU: p.SKU, Qty: 2}},
			})
			results <- orderErr
		}(i)
	}

	close(start)
	wg.Wait()
	close(results)

	succeeded := 0
	insufficient := 0
	for err := range results {
		if err == nil {
			succeeded++
			continue
		}
		var stockErr *InsufficientStockError
		if errors.As(err, &stockErr) {
			insufficient++
			continue
		}
		t.Fatalf("unexpected concurrent error: %v", err)
	}

	if succeeded != 2 || insufficient != 8 {
		t.Fatalf("expected 2 succeeded and 8 insufficient, got succeeded=%d insufficient=%d", succeeded, insufficient)
	}

	if got := getStock(t, pool, p.ID); got != 1 {
		t.Fatalf("expected remaining stock 1, got %d", got)
	}

	movements, err := s.ListMovements(ctx, p.ID)
	if err != nil {
		t.Fatalf("list movements: %v", err)
	}
	if len(movements) != 2 {
		t.Fatalf("expected 2 stock movements, got %d", len(movements))
	}
}