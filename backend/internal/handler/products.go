package handler

import (
	"net/http"
	"strconv"

	"stockflow/backend/internal/model"
)

func (h *Handler) listProducts(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	search := q.Get("search")
	filter := q.Get("filter")

	page, _ := strconv.Atoi(q.Get("page"))
	if page < 1 {
		page = 1
	}
	limit, _ := strconv.Atoi(q.Get("limit"))
	if limit < 1 || limit > 100 {
		limit = 20
	}

	products, total, err := h.store.ListProducts(r.Context(), search, filter, page, limit)
	if err != nil {
		handleStoreError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"data":  products,
		"page":  page,
		"limit": limit,
		"total": total,
	})
}

func (h *Handler) productSummary(w http.ResponseWriter, r *http.Request) {
	summary, err := h.store.GetProductSummary(r.Context())
	if err != nil {
		handleStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, summary)
}

func (h *Handler) createProduct(w http.ResponseWriter, r *http.Request) {
	var in model.CreateProductInput
	if err := decodeJSON(r, &in); err != nil {
		writeError(w, http.StatusBadRequest, apiError{
			Code:    "INVALID_BODY",
			Message: "invalid JSON body",
		})
		return
	}

	product, err := h.store.CreateProduct(r.Context(), in)
	if err != nil {
		handleStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, product)
}

func (h *Handler) updateProduct(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, apiError{
			Code:    "INVALID_ID",
			Message: "id must be a positive integer",
		})
		return
	}

	var in model.UpdateProductInput
	if err := decodeJSON(r, &in); err != nil {
		writeError(w, http.StatusBadRequest, apiError{
			Code:    "INVALID_BODY",
			Message: "invalid JSON body",
		})
		return
	}

	product, err := h.store.UpdateProduct(r.Context(), id, in)
	if err != nil {
		handleStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, product)
}

func (h *Handler) deleteProduct(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, apiError{
			Code:    "INVALID_ID",
			Message: "id must be a positive integer",
		})
		return
	}

	if err := h.store.DeleteProduct(r.Context(), id); err != nil {
		handleStoreError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) adjustStock(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, apiError{
			Code:    "INVALID_ID",
			Message: "id must be a positive integer",
		})
		return
	}

	var in model.AdjustStockInput
	if err := decodeJSON(r, &in); err != nil {
		writeError(w, http.StatusBadRequest, apiError{
			Code:    "INVALID_BODY",
			Message: "invalid JSON body",
		})
		return
	}

	product, movement, err := h.store.AdjustStock(r.Context(), id, in)
	if err != nil {
		handleStoreError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"product":  product,
		"movement": movement,
	})
}

func (h *Handler) listMovements(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, apiError{
			Code:    "INVALID_ID",
			Message: "id must be a positive integer",
		})
		return
	}

	movements, err := h.store.ListMovements(r.Context(), id)
	if err != nil {
		handleStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"data": movements,
	})
}