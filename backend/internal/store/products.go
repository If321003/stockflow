 package store

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"stockflow/backend/internal/model"

	"github.com/jackc/pgx/v5"
)

func (s *Store) ListProducts(ctx context.Context, search, filter string, page, limit int) ([]model.Product, int, error) {
	if page < 1 {
		page = 1
	}
	if limit < 1 || limit > 100 {
		limit = 20
	}

	var conds []string
	var args []any
	idx := 1

	if q := strings.TrimSpace(search); q != "" {
		conds = append(conds, fmt.Sprintf("(p.sku ILIKE $%d OR p.name ILIKE $%d)", idx, idx))
		args = append(args, "%"+q+"%")
		idx++
	}

	switch filter {
	case "active":
		conds = append(conds, "p.is_active = TRUE")
	case "inactive":
		conds = append(conds, "p.is_active = FALSE")
	case "low_stock":
		conds = append(conds, "p.stock <= 10")
	}

	where := ""
	if len(conds) > 0 {
		where = "WHERE " + strings.Join(conds, " AND ")
	}

	var total int
	countQuery := fmt.Sprintf("SELECT COUNT(*) FROM products p %s", where)
	if err := s.pool.QueryRow(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	offset := (page - 1) * limit
	listQuery := fmt.Sprintf(`
		SELECT
			p.id, p.sku, p.name, p.price, p.stock, p.is_active, p.created_at,
			(SELECT MAX(sm.created_at) FROM stock_movements sm WHERE sm.product_id = p.id) AS last_movement_at
		FROM products p
		%s
		ORDER BY p.id DESC
		LIMIT $%d OFFSET $%d
	`, where, idx, idx+1)

	rows, err := s.pool.Query(ctx, listQuery, append(args, limit, offset)...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	products := make([]model.Product, 0)
	for rows.Next() {
		var p model.Product
		err := rows.Scan(
			&p.ID, &p.SKU, &p.Name, &p.Price, &p.Stock, &p.IsActive, &p.CreatedAt, &p.LastMovementAt,
		)
		if err != nil {
			return nil, 0, err
		}
		products = append(products, p)
	}

	return products, total, rows.Err()
}

func (s *Store) GetProductSummary(ctx context.Context) (model.ProductSummary, error) {
	var sum model.ProductSummary
	err := s.pool.QueryRow(ctx, `
		SELECT
			COUNT(*),
			COUNT(*) FILTER (WHERE is_active = TRUE),
			COALESCE(SUM(price::bigint * stock::bigint), 0),
			COUNT(*) FILTER (WHERE stock <= 10)
		FROM products
	`).Scan(&sum.TotalProducts, &sum.ActiveProducts, &sum.TotalStockValue, &sum.LowStockCount)
	return sum, err
}

func (s *Store) CreateProduct(ctx context.Context, in model.CreateProductInput) (model.Product, error) {
	sku := strings.TrimSpace(in.SKU)
	name := strings.TrimSpace(in.Name)
	if sku == "" {
		return model.Product{}, &ValidationError{Message: "sku is required"}
	}
	if name == "" {
		return model.Product{}, &ValidationError{Message: "name is required"}
	}
	if in.Price < 0 {
		return model.Product{}, &ValidationError{Message: "price must be >= 0"}
	}
	if in.Stock < 0 {
		return model.Product{}, &ValidationError{Message: "stock must be >= 0"}
	}

	isActive := true
	if in.IsActive != nil {
		isActive = *in.IsActive
	}

	var p model.Product
	err := s.pool.QueryRow(ctx, `
		INSERT INTO products (sku, name, price, stock, is_active)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id, sku, name, price, stock, is_active, created_at
	`, sku, name, in.Price, in.Stock, isActive).Scan(
		&p.ID, &p.SKU, &p.Name, &p.Price, &p.Stock, &p.IsActive, &p.CreatedAt,
	)
	if err != nil {
		if isPgCode(err, "23505") {
			return model.Product{}, ErrDuplicateSKU
		}
		return model.Product{}, err
	}
	return p, nil
}

func (s *Store) UpdateProduct(ctx context.Context, id int, in model.UpdateProductInput) (model.Product, error) {
	sku := strings.TrimSpace(in.SKU)
	name := strings.TrimSpace(in.Name)
	if sku == "" {
		return model.Product{}, &ValidationError{Message: "sku is required"}
	}
	if name == "" {
		return model.Product{}, &ValidationError{Message: "name is required"}
	}
	if in.Price < 0 {
		return model.Product{}, &ValidationError{Message: "price must be >= 0"}
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return model.Product{}, err
	}
	defer tx.Rollback(ctx)

	var currentActive bool
	err = tx.QueryRow(ctx, `SELECT is_active FROM products WHERE id = $1 FOR UPDATE`, id).Scan(&currentActive)
	if errors.Is(err, pgx.ErrNoRows) {
		return model.Product{}, ErrNotFound
	}
	if err != nil {
		return model.Product{}, err
	}

	isActive := currentActive
	if in.IsActive != nil {
		isActive = *in.IsActive
	}

	var p model.Product
	err = tx.QueryRow(ctx, `
		UPDATE products
		SET sku = $1, name = $2, price = $3, is_active = $4
		WHERE id = $5
		RETURNING id, sku, name, price, stock, is_active, created_at,
			(SELECT MAX(sm.created_at) FROM stock_movements sm WHERE sm.product_id = products.id)
	`, sku, name, in.Price, isActive, id).Scan(
		&p.ID, &p.SKU, &p.Name, &p.Price, &p.Stock, &p.IsActive, &p.CreatedAt, &p.LastMovementAt,
	)
	if err != nil {
		if isPgCode(err, "23505") {
			return model.Product{}, ErrDuplicateSKU
		}
		if isPgCode(err, "23503") {
			return model.Product{}, &ValidationError{Message: "cannot change SKU because it is referenced by existing orders"}
		}
		return model.Product{}, err
	}

	if err := tx.Commit(ctx); err != nil {
		return model.Product{}, err
	}
	return p, nil
}

func (s *Store) DeleteProduct(ctx context.Context, id int) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	var existingID int
	err = tx.QueryRow(ctx, `SELECT id FROM products WHERE id = $1 FOR UPDATE`, id).Scan(&existingID)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}

	var hasHistory bool
	err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM stock_movements WHERE product_id = $1)`, id).Scan(&hasHistory)
	if err != nil {
		return err
	}
	if hasHistory {
		return ErrProductHasHistory
	}

	if _, err := tx.Exec(ctx, `DELETE FROM products WHERE id = $1`, id); err != nil {
		if isPgCode(err, "23503") {
			return ErrProductHasHistory
		}
		return err
	}

	return tx.Commit(ctx)
}

func (s *Store) AdjustStock(ctx context.Context, productID int, in model.AdjustStockInput) (model.Product, model.StockMovement, error) {
	if in.Type != "in" && in.Type != "out" {
		return model.Product{}, model.StockMovement{}, &ValidationError{Message: "type must be 'in' or 'out'"}
	}
	if in.Qty <= 0 {
		return model.Product{}, model.StockMovement{}, &ValidationError{Message: "qty must be > 0"}
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return model.Product{}, model.StockMovement{}, err
	}
	defer tx.Rollback(ctx)

	var currentStock int
	err = tx.QueryRow(ctx, `SELECT stock FROM products WHERE id = $1 FOR UPDATE`, productID).Scan(&currentStock)
	if errors.Is(err, pgx.ErrNoRows) {
		return model.Product{}, model.StockMovement{}, ErrNotFound
	}
	if err != nil {
		return model.Product{}, model.StockMovement{}, err
	}

	var updated model.Product
	if in.Type == "out" {
		if currentStock < in.Qty {
			return model.Product{}, model.StockMovement{}, &InsufficientStockError{Available: currentStock}
		}
		err = tx.QueryRow(ctx, `
			UPDATE products
			SET stock = stock - $1
			WHERE id = $2 AND stock >= $1
			RETURNING id, sku, name, price, stock, is_active, created_at
		`, in.Qty, productID).Scan(
			&updated.ID, &updated.SKU, &updated.Name, &updated.Price, &updated.Stock, &updated.IsActive, &updated.CreatedAt,
		)
		if errors.Is(err, pgx.ErrNoRows) {
			return model.Product{}, model.StockMovement{}, &InsufficientStockError{Available: currentStock}
		}
	} else {
		err = tx.QueryRow(ctx, `
			UPDATE products
			SET stock = stock + $1
			WHERE id = $2
			RETURNING id, sku, name, price, stock, is_active, created_at
		`, in.Qty, productID).Scan(
			&updated.ID, &updated.SKU, &updated.Name, &updated.Price, &updated.Stock, &updated.IsActive, &updated.CreatedAt,
		)
	}
	if err != nil {
		return model.Product{}, model.StockMovement{}, err
	}

	var mv model.StockMovement
	err = tx.QueryRow(ctx, `
		INSERT INTO stock_movements (product_id, type, qty, note)
		VALUES ($1, $2, $3, $4)
		RETURNING id, product_id, type, qty, COALESCE(note, ''), created_at
	`, productID, in.Type, in.Qty, strings.TrimSpace(in.Note)).Scan(
		&mv.ID, &mv.ProductID, &mv.Type, &mv.Qty, &mv.Note, &mv.CreatedAt,
	)
	if err != nil {
		return model.Product{}, model.StockMovement{}, err
	}

	if err := tx.Commit(ctx); err != nil {
		return model.Product{}, model.StockMovement{}, err
	}

	updated.LastMovementAt = &mv.CreatedAt
	return updated, mv, nil
}

func (s *Store) ListMovements(ctx context.Context, productID int) ([]model.StockMovement, error) {
	var exists bool
	if err := s.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM products WHERE id = $1)`, productID).Scan(&exists); err != nil {
		return nil, err
	}
	if !exists {
		return nil, ErrNotFound
	}

	rows, err := s.pool.Query(ctx, `
		SELECT id, product_id, type, qty, COALESCE(note, ''), created_at
		FROM stock_movements
		WHERE product_id = $1
		ORDER BY created_at DESC, id DESC
	`, productID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	movements := make([]model.StockMovement, 0)
	for rows.Next() {
		var m model.StockMovement
		if err := rows.Scan(&m.ID, &m.ProductID, &m.Type, &m.Qty, &m.Note, &m.CreatedAt); err != nil {
			return nil, err
		}
		movements = append(movements, m)
	}
	return movements, rows.Err()
}