export interface Product {
  id: number
  sku: string
  name: string
  price: number
  stock: number
  is_active: boolean
  created_at: string
  last_movement_at: string | null
}

export interface ProductSummary {
  total_products: number
  active_products: number
  total_stock_value: number
  low_stock_count: number
}

export interface StockMovement {
  id: number
  product_id: number
  type: 'in' | 'out'
  qty: number
  note: string
  created_at: string
}

export interface SalesOrderItem {
  id: number
  order_id: number
  sku: string
  qty: number
  unit_price: number
}

export interface SalesOrder {
  id: number
  customer: string
  status: 'CREATED' | 'CANCELLED'
  total: number
  created_at: string
  cancelled_at: string | null
  items: SalesOrderItem[]
}

export interface ProductListResponse {
  data: Product[]
  page: number
  limit: number
  total: number
}

export class ApiError extends Error {
  code: string
  status: number
  sku?: string
  available?: number

  constructor(status: number, code: string, message: string, sku?: string, available?: number) {
    super(message)
    this.status = status
    this.code = code
    this.sku = sku
    this.available = available
  }
}

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const res = await fetch(path, {
    headers: { 'Content-Type': 'application/json', ...(init?.headers ?? {}) },
    ...init,
  })

  if (res.status === 204) {
    return undefined as T
  }

  const body = await res.json().catch(() => null)
  if (!res.ok) {
    const err = body?.error
    throw new ApiError(
      res.status,
      err?.code ?? 'REQUEST_FAILED',
      err?.message ?? `Request failed (${res.status})`,
      err?.sku,
      err?.available,
    )
  }

  return body as T
}

export const api = {
  listProducts(params: { search?: string; filter?: string; page?: number; limit?: number }) {
    const qs = new URLSearchParams()
    if (params.search) qs.set('search', params.search)
    if (params.filter && params.filter !== 'all') qs.set('filter', params.filter)
    if (params.page) qs.set('page', String(params.page))
    if (params.limit) qs.set('limit', String(params.limit))
    return request<ProductListResponse>(`/api/products?${qs.toString()}`)
  },

  getSummary() {
    return request<ProductSummary>('/api/products/summary')
  },

  createProduct(payload: { sku: string; name: string; price: number; stock: number; is_active?: boolean }) {
    return request<Product>('/api/products', {
      method: 'POST',
      body: JSON.stringify(payload),
    })
  },

  updateProduct(id: number, payload: { sku: string; name: string; price: number; is_active?: boolean }) {
    return request<Product>(`/api/products/${id}`, {
      method: 'PUT',
      body: JSON.stringify(payload),
    })
  },

  deleteProduct(id: number) {
    return request<void>(`/api/products/${id}`, { method: 'DELETE' })
  },

  adjustStock(id: number, payload: { type: 'in' | 'out'; qty: number; note: string }) {
    return request<{ product: Product; movement: StockMovement }>(`/api/products/${id}/stock`, {
      method: 'POST',
      body: JSON.stringify(payload),
    })
  },

  listMovements(id: number) {
    return request<{ data: StockMovement[] }>(`/api/products/${id}/movements`)
  },

  listOrders() {
    return request<{ data: SalesOrder[] }>('/api/orders')
  },

  getOrder(id: number) {
    return request<SalesOrder>(`/api/orders/${id}`)
  },

  createOrder(payload: { customer: string; items: { sku: string; qty: number }[] }) {
    return request<SalesOrder>('/api/orders', {
      method: 'POST',
      body: JSON.stringify(payload),
    })
  },

  cancelOrder(id: number) {
    return request<SalesOrder>(`/api/orders/${id}/cancel`, { method: 'POST' })
  },
}