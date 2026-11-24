package store

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	"stockflow/backend/internal/model"

	"github.com/jackc/pgx/v5"
)

func (s *Store) CreateOrder(ctx context.Context, in model.CreateOrderInput) (model.SalesOrder, error) {
	customer := strings.TrimSpace(in.Customer)
	if customer == "" {
		return model.SalesOrder{}, &ValidationError{Message: "customer is required"}
	}
	if len(in.Items) == 0 {
		return model.SalesOrder{}, &ValidationError{Message: "items must not be empty"}
	}

	seen := make(map[string]bool, len(in.Items))
	skus := make([]string, 0, len(in.Items))
	items := make([]model.OrderItemInput, 0, len(in.Items))

	for _, item := range in.Items {
		sku := strings.TrimSpace(item.SKU)
		if sku == "" {
			return model.SalesOrder{}, &ValidationError{Message: "item sku is required"}
		}
		if item.Qty <= 0 {
			return model.SalesOrder{}, &ValidationError{Message: fmt.Sprintf("qty for %s must be > 0", sku)}
		}
		if seen[sku] {
			return model.SalesOrder{}, &ValidationError{Message: fmt.Sprintf("duplicate sku in order: %s", sku)}
		}
		seen[sku] = true
		skus = append(skus, sku)
		items = append(items, model.OrderItemInput{SKU: sku, Qty: item.Qty})
	}

	// Urutkan SKU sebelum lock baris supaya dua order paralel tidak saling tunggu (deadlock)
	sort.Strings(skus)

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return model.SalesOrder{}, err
	}
	defer tx.Rollback(ctx)

	type lockedProduct struct {
		id    int
		sku   string
		price int
		stock int
	}

	rows, err := tx.Query(ctx, `
		SELECT id, sku, price, stock
		FROM products
		WHERE sku = ANY($1)
		ORDER BY sku
		FOR UPDATE
	`, skus)
	if err != nil {
		return model.SalesOrder{}, err
	}

	bySKU := make(map[string]lockedProduct, len(skus))
	for rows.Next() {
		var p lockedProduct
		if err := rows.Scan(&p.id, &p.sku, &p.price, &p.stock); err != nil {
			rows.Close()
			return model.SalesOrder{}, err
		}
		bySKU[p.sku] = p
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return model.SalesOrder{}, err
	}

	var missing []string
	for _, item := range items {
		if _, ok := bySKU[item.SKU]; !ok {
			missing = append(missing, item.SKU)
		}
	}
	if len(missing) > 0 {
		return model.SalesOrder{}, &ValidationError{
			Message: fmt.Sprintf("unknown sku: %s", strings.Join(missing, ", ")),
		}
	}

	total := 0
	for _, item := range items {
		p := bySKU[item.SKU]
		if p.stock < item.Qty {
			return model.SalesOrder{}, &InsufficientStockError{
				SKU:       p.sku,
				Available: p.stock,
			}
		}
		total += p.price * item.Qty
	}

	var order model.SalesOrder
	order.Customer = customer
	order.Status = "CREATED"
	order.Total = total

	err = tx.QueryRow(ctx, `
		INSERT INTO sales_orders (customer, status, total)
		VALUES ($1, 'CREATED', $2)
		RETURNING id, created_at
	`, customer, total).Scan(&order.ID, &order.CreatedAt)
	if err != nil {
		return model.SalesOrder{}, err
	}

	note := fmt.Sprintf("Sales order #%d", order.ID)
	order.Items = make([]model.SalesOrderItem, 0, len(items))

	for _, item := range items {
		p := bySKU[item.SKU]

		tag, err := tx.Exec(ctx, `
			UPDATE products
			SET stock = stock - $1
			WHERE id = $2 AND stock >= $1
		`, item.Qty, p.id)
		if err != nil {
			return model.SalesOrder{}, err
		}
		if tag.RowsAffected() == 0 {
			return model.SalesOrder{}, &InsufficientStockError{
				SKU:       p.sku,
				Available: p.stock,
			}
		}

		var oi model.SalesOrderItem
		err = tx.QueryRow(ctx, `
			INSERT INTO sales_order_items (order_id, sku, qty, unit_price)
			VALUES ($1, $2, $3, $4)
			RETURNING id, order_id, sku, qty, unit_price
		`, order.ID, p.sku, item.Qty, p.price).Scan(
			&oi.ID, &oi.OrderID, &oi.SKU, &oi.Qty, &oi.UnitPrice,
		)
		if err != nil {
			return model.SalesOrder{}, err
		}
		order.Items = append(order.Items, oi)

		if _, err := tx.Exec(ctx, `
			INSERT INTO stock_movements (product_id, type, qty, note)
			VALUES ($1, 'out', $2, $3)
		`, p.id, item.Qty, note); err != nil {
			return model.SalesOrder{}, err
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return model.SalesOrder{}, err
	}
	return order, nil
}

func (s *Store) GetOrder(ctx context.Context, id int) (model.SalesOrder, error) {
	var order model.SalesOrder
	err := s.pool.QueryRow(ctx, `
		SELECT id, customer, status, total, created_at, cancelled_at
		FROM sales_orders
		WHERE id = $1
	`, id).Scan(&order.ID, &order.Customer, &order.Status, &order.Total, &order.CreatedAt, &order.CancelledAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return model.SalesOrder{}, ErrNotFound
	}
	if err != nil {
		return model.SalesOrder{}, err
	}

	rows, err := s.pool.Query(ctx, `
		SELECT id, order_id, sku, qty, unit_price
		FROM sales_order_items
		WHERE order_id = $1
		ORDER BY id
	`, id)
	if err != nil {
		return model.SalesOrder{}, err
	}
	defer rows.Close()

	order.Items = make([]model.SalesOrderItem, 0)
	for rows.Next() {
		var item model.SalesOrderItem
		if err := rows.Scan(&item.ID, &item.OrderID, &item.SKU, &item.Qty, &item.UnitPrice); err != nil {
			return model.SalesOrder{}, err
		}
		order.Items = append(order.Items, item)
	}
	return order, rows.Err()
}

func (s *Store) ListOrders(ctx context.Context) ([]model.SalesOrder, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, customer, status, total, created_at, cancelled_at
		FROM sales_orders
		ORDER BY id DESC
		LIMIT 50
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	orders := make([]model.SalesOrder, 0)
	for rows.Next() {
		var o model.SalesOrder
		if err := rows.Scan(&o.ID, &o.Customer, &o.Status, &o.Total, &o.CreatedAt, &o.CancelledAt); err != nil {
			return nil, err
		}
		o.Items = []model.SalesOrderItem{}
		orders = append(orders, o)
	}
	return orders, rows.Err()
}

func (s *Store) CancelOrder(ctx context.Context, id int) (model.SalesOrder, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return model.SalesOrder{}, err
	}
	defer tx.Rollback(ctx)

	var order model.SalesOrder
	err = tx.QueryRow(ctx, `
		SELECT id, customer, status, total, created_at, cancelled_at
		FROM sales_orders
		WHERE id = $1
		FOR UPDATE
	`, id).Scan(&order.ID, &order.Customer, &order.Status, &order.Total, &order.CreatedAt, &order.CancelledAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return model.SalesOrder{}, ErrNotFound
	}
	if err != nil {
		return model.SalesOrder{}, err
	}

	if order.Status == "CANCELLED" {
		return model.SalesOrder{}, ErrAlreadyCancelled
	}

	itemRows, err := tx.Query(ctx, `
		SELECT id, order_id, sku, qty, unit_price
		FROM sales_order_items
		WHERE order_id = $1
		ORDER BY sku
	`, id)
	if err != nil {
		return model.SalesOrder{}, err
	}

	order.Items = make([]model.SalesOrderItem, 0)
	skus := make([]string, 0)
	for itemRows.Next() {
		var item model.SalesOrderItem
		if err := itemRows.Scan(&item.ID, &item.OrderID, &item.SKU, &item.Qty, &item.UnitPrice); err != nil {
			itemRows.Close()
			return model.SalesOrder{}, err
		}
		order.Items = append(order.Items, item)
		skus = append(skus, item.SKU)
	}
	itemRows.Close()
	if err := itemRows.Err(); err != nil {
		return model.SalesOrder{}, err
	}

	prodRows, err := tx.Query(ctx, `
		SELECT id, sku
		FROM products
		WHERE sku = ANY($1)
		ORDER BY sku
		FOR UPDATE
	`, skus)
	if err != nil {
		return model.SalesOrder{}, err
	}

	idBySKU := make(map[string]int, len(skus))
	for prodRows.Next() {
		var pid int
		var sku string
		if err := prodRows.Scan(&pid, &sku); err != nil {
			prodRows.Close()
			return model.SalesOrder{}, err
		}
		idBySKU[sku] = pid
	}
	prodRows.Close()
	if err := prodRows.Err(); err != nil {
		return model.SalesOrder{}, err
	}

	note := fmt.Sprintf("Cancel order #%d", order.ID)
	for _, item := range order.Items {
		pid := idBySKU[item.SKU]
		if _, err := tx.Exec(ctx, `UPDATE products SET stock = stock + $1 WHERE id = $2`, item.Qty, pid); err != nil {
			return model.SalesOrder{}, err
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO stock_movements (product_id, type, qty, note)
			VALUES ($1, 'in', $2, $3)
		`, pid, item.Qty, note); err != nil {
			return model.SalesOrder{}, err
		}
	}

	err = tx.QueryRow(ctx, `
		UPDATE sales_orders
		SET status = 'CANCELLED', cancelled_at = now()
		WHERE id = $1
		RETURNING status, cancelled_at
	`, id).Scan(&order.Status, &order.CancelledAt)
	if err != nil {
		return model.SalesOrder{}, err
	}

	if err := tx.Commit(ctx); err != nil {
		return model.SalesOrder{}, err
	}
	return order, nil
}