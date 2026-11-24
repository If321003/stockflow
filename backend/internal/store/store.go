package store

import (
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrNotFound          = errors.New("record not found")
	ErrDuplicateSKU      = errors.New("sku already exists")
	ErrProductHasHistory = errors.New("product has movement history")
	ErrAlreadyCancelled  = errors.New("order already cancelled")
)

type ValidationError struct {
	Message string
}

func (e *ValidationError) Error() string {
	return e.Message
}

type InsufficientStockError struct {
	SKU       string
	Available int
}

func (e *InsufficientStockError) Error() string {
	if e.SKU != "" {
		return fmt.Sprintf("insufficient stock for %s (available: %d)", e.SKU, e.Available)
	}
	return fmt.Sprintf("insufficient stock (available: %d)", e.Available)
}

type Store struct {
	pool *pgxpool.Pool
}

func New(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
}

func isPgCode(err error, code string) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == code
}