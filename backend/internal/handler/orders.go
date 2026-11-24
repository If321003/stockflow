package handler

import (
	"net/http"

	"stockflow/backend/internal/model"
)

func (h *Handler) listOrders(w http.ResponseWriter, r *http.Request) {
	orders, err := h.store.ListOrders(r.Context())
	if err != nil {
		handleStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"data": orders,
	})
}

func (h *Handler) createOrder(w http.ResponseWriter, r *http.Request) {
	var in model.CreateOrderInput
	if err := decodeJSON(r, &in); err != nil {
		writeError(w, http.StatusBadRequest, apiError{
			Code:    "INVALID_BODY",
			Message: "invalid JSON body",
		})
		return
	}

	order, err := h.store.CreateOrder(r.Context(), in)
	if err != nil {
		handleStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, order)
}

func (h *Handler) getOrder(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, apiError{
			Code:    "INVALID_ID",
			Message: "id must be a positive integer",
		})
		return
	}

	order, err := h.store.GetOrder(r.Context(), id)
	if err != nil {
		handleStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, order)
}

func (h *Handler) cancelOrder(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, apiError{
			Code:    "INVALID_ID",
			Message: "id must be a positive integer",
		})
		return
	}

	order, err := h.store.CancelOrder(r.Context(), id)
	if err != nil {
		handleStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, order)
}