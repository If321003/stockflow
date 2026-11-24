package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"stockflow/backend/internal/model"
	"stockflow/backend/internal/store"

	"github.com/jackc/pgx/v5/pgxpool"
)

func setupTestRouter(t *testing.T) (http.Handler, *pgxpool.Pool) {
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

	h := New(store.New(pool))
	return h.Routes(), pool
}

func cleanupTestData(t *testing.T, pool *pgxpool.Pool, productIDs []int, skus []string) {
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

func doJSON(t *testing.T, router http.Handler, method, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var buf bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&buf).Encode(body); err != nil {
			t.Fatalf("encode request: %v", err)
		}
	}
	req := httptest.NewRequest(method, path, &buf)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

func TestProductHTTPLifecycle(t *testing.T) {
	router, pool := setupTestRouter(t)
	sku := fmt.Sprintf("HTTP-PRD-%d", time.Now().UnixNano())

	rec := doJSON(t, router, http.MethodPost, "/api/products", map[string]any{
		"sku":   sku,
		"name":  "HTTP Test Keyboard",
		"price": 150000,
		"stock": 5,
	})
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
	}

	var created model.Product
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode created product: %v", err)
	}

	t.Cleanup(func() {
		cleanupTestData(t, pool, []int{created.ID}, []string{sku})
	})

	// Duplicate SKU -> 409 DUPLICATE_SKU
	rec = doJSON(t, router, http.MethodPost, "/api/products", map[string]any{
		"sku":   sku,
		"name":  "Duplicate",
		"price": 100000,
		"stock": 1,
	})
	if rec.Code != http.StatusConflict {
		t.Fatalf("expected 409 on duplicate sku, got %d", rec.Code)
	}
	var errResp errorEnvelope
	_ = json.Unmarshal(rec.Body.Bytes(), &errResp)
	if errResp.Error.Code != "DUPLICATE_SKU" {
		t.Fatalf("expected DUPLICATE_SKU, got %q", errResp.Error.Code)
	}

	// Adjust out exceeding stock -> 409 INSUFFICIENT_STOCK
	rec = doJSON(t, router, http.MethodPost, fmt.Sprintf("/api/products/%d/stock", created.ID), map[string]any{
		"type": "out",
		"qty":  10,
		"note": "too much",
	})
	if rec.Code != http.StatusConflict {
		t.Fatalf("expected 409 on insufficient stock, got %d", rec.Code)
	}
	errResp = errorEnvelope{}
	_ = json.Unmarshal(rec.Body.Bytes(), &errResp)
	if errResp.Error.Code != "INSUFFICIENT_STOCK" || errResp.Error.Available == nil || *errResp.Error.Available != 5 {
		t.Fatalf("unexpected error payload: %+v", errResp.Error)
	}

	// Adjust in -> 200 OK
	rec = doJSON(t, router, http.MethodPost, fmt.Sprintf("/api/products/%d/stock", created.ID), map[string]any{
		"type": "in",
		"qty":  3,
		"note": "restock",
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 on stock in, got %d: %s", rec.Code, rec.Body.String())
	}

	// Delete product with movement history -> 409 PRODUCT_HAS_HISTORY
	rec = doJSON(t, router, http.MethodDelete, fmt.Sprintf("/api/products/%d", created.ID), nil)
	if rec.Code != http.StatusConflict {
		t.Fatalf("expected 409 on delete with history, got %d", rec.Code)
	}
	errResp = errorEnvelope{}
	_ = json.Unmarshal(rec.Body.Bytes(), &errResp)
	if errResp.Error.Code != "PRODUCT_HAS_HISTORY" {
		t.Fatalf("expected PRODUCT_HAS_HISTORY, got %q", errResp.Error.Code)
	}
}

func TestOrderHTTPEndpoints(t *testing.T) {
	router, pool := setupTestRouter(t)
	sku := fmt.Sprintf("HTTP-ORD-%d", time.Now().UnixNano())

	rec := doJSON(t, router, http.MethodPost, "/api/products", map[string]any{
		"sku":   sku,
		"name":  "HTTP Test Monitor",
		"price": 200000,
		"stock": 5,
	})
	if rec.Code != http.StatusCreated {
		t.Fatalf("create product failed: %d %s", rec.Code, rec.Body.String())
	}
	var prod model.Product
	_ = json.Unmarshal(rec.Body.Bytes(), &prod)

	t.Cleanup(func() {
		cleanupTestData(t, pool, []int{prod.ID}, []string{sku})
	})

	// Empty customer -> 400 VALIDATION_ERROR
	rec = doJSON(t, router, http.MethodPost, "/api/orders", map[string]any{
		"customer": "",
		"items":    []map[string]any{{"sku": sku, "qty": 1}},
	})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 on empty customer, got %d", rec.Code)
	}

	// Create valid order -> 201 Created
	rec = doJSON(t, router, http.MethodPost, "/api/orders", map[string]any{
		"customer": "Andi",
		"items":    []map[string]any{{"sku": sku, "qty": 3}},
	})
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201 on create order, got %d: %s", rec.Code, rec.Body.String())
	}
	var order model.SalesOrder
	_ = json.Unmarshal(rec.Body.Bytes(), &order)
	if order.Total != 600000 || order.Status != "CREATED" {
		t.Fatalf("unexpected order response: %+v", order)
	}

	// Insufficient stock -> 409 with sku & available=2
	rec = doJSON(t, router, http.MethodPost, "/api/orders", map[string]any{
		"customer": "Siti",
		"items":    []map[string]any{{"sku": sku, "qty": 4}},
	})
	if rec.Code != http.StatusConflict {
		t.Fatalf("expected 409 on insufficient stock, got %d", rec.Code)
	}
	var errResp errorEnvelope
	_ = json.Unmarshal(rec.Body.Bytes(), &errResp)
	if errResp.Error.Code != "INSUFFICIENT_STOCK" || errResp.Error.SKU != sku || errResp.Error.Available == nil || *errResp.Error.Available != 2 {
		t.Fatalf("unexpected error payload: %+v", errResp.Error)
	}

	// Get order -> 200 OK
	rec = doJSON(t, router, http.MethodGet, fmt.Sprintf("/api/orders/%d", order.ID), nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 on get order, got %d", rec.Code)
	}

	// Cancel order -> 200 OK
	rec = doJSON(t, router, http.MethodPost, fmt.Sprintf("/api/orders/%d/cancel", order.ID), nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 on cancel order, got %d: %s", rec.Code, rec.Body.String())
	}

	// Cancel twice -> 409 ORDER_ALREADY_CANCELLED
	rec = doJSON(t, router, http.MethodPost, fmt.Sprintf("/api/orders/%d/cancel", order.ID), nil)
	if rec.Code != http.StatusConflict {
		t.Fatalf("expected 409 on double cancel, got %d", rec.Code)
	}
	errResp = errorEnvelope{}
	_ = json.Unmarshal(rec.Body.Bytes(), &errResp)
	if errResp.Error.Code != "ORDER_ALREADY_CANCELLED" {
		t.Fatalf("expected ORDER_ALREADY_CANCELLED, got %q", errResp.Error.Code)
	}

	// Unknown order -> 404 NOT_FOUND
	rec = doJSON(t, router, http.MethodGet, "/api/orders/99999999", nil)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 on unknown order, got %d", rec.Code)
	}
}