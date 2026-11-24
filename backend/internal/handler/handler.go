package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"stockflow/backend/internal/store"
)

type Handler struct {
	store *store.Store
}

func New(s *store.Store) *Handler {
	return &Handler{store: s}
}

func (h *Handler) Routes() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /api/products", h.listProducts)
	mux.HandleFunc("GET /api/products/summary", h.productSummary)
	mux.HandleFunc("POST /api/products", h.createProduct)
	mux.HandleFunc("PUT /api/products/{id}", h.updateProduct)
	mux.HandleFunc("DELETE /api/products/{id}", h.deleteProduct)
	mux.HandleFunc("POST /api/products/{id}/stock", h.adjustStock)
	mux.HandleFunc("GET /api/products/{id}/movements", h.listMovements)

	mux.HandleFunc("GET /api/orders", h.listOrders)
	mux.HandleFunc("POST /api/orders", h.createOrder)
	mux.HandleFunc("GET /api/orders/{id}", h.getOrder)
	mux.HandleFunc("POST /api/orders/{id}/cancel", h.cancelOrder)

	return withCORS(mux)
}

type apiError struct {
	Code      string `json:"code"`
	Message   string `json:"message,omitempty"`
	SKU       string `json:"sku,omitempty"`
	Available *int   `json:"available,omitempty"`
}

type errorEnvelope struct {
	Error apiError `json:"error"`
}

func writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}

func writeError(w http.ResponseWriter, status int, err apiError) {
	writeJSON(w, status, errorEnvelope{Error: err})
}

func handleStoreError(w http.ResponseWriter, err error) {
	var valErr *store.ValidationError
	if errors.As(err, &valErr) {
		writeError(w, http.StatusBadRequest, apiError{
			Code:    "VALIDATION_ERROR",
			Message: valErr.Message,
		})
		return
	}

	var stockErr *store.InsufficientStockError
	if errors.As(err, &stockErr) {
		avail := stockErr.Available
		writeError(w, http.StatusConflict, apiError{
			Code:      "INSUFFICIENT_STOCK",
			Message:   stockErr.Error(),
			SKU:       stockErr.SKU,
			Available: &avail,
		})
		return
	}

	switch {
	case errors.Is(err, store.ErrNotFound):
		writeError(w, http.StatusNotFound, apiError{
			Code:    "NOT_FOUND",
			Message: "resource not found",
		})
	case errors.Is(err, store.ErrDuplicateSKU):
		writeError(w, http.StatusConflict, apiError{
			Code:    "DUPLICATE_SKU",
			Message: "SKU already exists",
		})
	case errors.Is(err, store.ErrProductHasHistory):
		writeError(w, http.StatusConflict, apiError{
			Code:    "PRODUCT_HAS_HISTORY",
			Message: "product has movement history",
		})
	case errors.Is(err, store.ErrAlreadyCancelled):
		writeError(w, http.StatusConflict, apiError{
			Code:    "ORDER_ALREADY_CANCELLED",
			Message: "order already cancelled",
		})
	default:
		writeError(w, http.StatusInternalServerError, apiError{
			Code:    "INTERNAL_ERROR",
			Message: "internal server error",
		})
	}
}

func parseID(r *http.Request) (int, error) {
	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil || id <= 0 {
		return 0, errors.New("invalid id")
	}
	return id, nil
}

func decodeJSON(r *http.Request, dst any) error {
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	return dec.Decode(dst)
}

func withCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Idempotency-Key")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}