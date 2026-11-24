import { useCallback, useEffect, useState } from 'react'
import { api, ApiError, type Product, type SalesOrder } from '../api'

function formatIDR(amount: number) {
  return new Intl.NumberFormat('id-ID', {
    style: 'currency',
    currency: 'IDR',
    maximumFractionDigits: 0,
  }).format(amount)
}

function formatDate(iso: string | null) {
  if (!iso) return '-'
  return new Date(iso).toLocaleString('id-ID', {
    dateStyle: 'short',
    timeStyle: 'short',
  })
}

interface DraftItem {
  sku: string
  qty: number
}

export function OrdersTab() {
  const [orders, setOrders] = useState<SalesOrder[]>([])
  const [products, setProducts] = useState<Product[]>([])
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)

  // Create order modal
  const [createOpen, setCreateOpen] = useState(false)
  const [customer, setCustomer] = useState('')
  const [items, setItems] = useState<DraftItem[]>([{ sku: '', qty: 1 }])
  const [formError, setFormError] = useState<string | null>(null)
  const [submitting, setSubmitting] = useState(false)

  // Detail modal
  const [detail, setDetail] = useState<SalesOrder | null>(null)
  const [detailLoading, setDetailLoading] = useState(false)

  const loadOrders = useCallback(async () => {
    setLoading(true)
    setError(null)
    try {
      const [ordRes, prodRes] = await Promise.all([
        api.listOrders(),
        api.listProducts({ limit: 100 }),
      ])
      setOrders(ordRes.data)
      setProducts(prodRes.data)
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to load orders')
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => {
    void loadOrders()
  }, [loadOrders])

  function openCreate() {
    const firstSKU = products.find((p) => p.is_active && p.stock > 0)?.sku ?? products[0]?.sku ?? ''
    setCustomer('')
    setItems([{ sku: firstSKU, qty: 1 }])
    setFormError(null)
    setCreateOpen(true)
  }

  function updateItem(idx: number, patch: Partial<DraftItem>) {
    setItems((prev) => prev.map((it, i) => (i === idx ? { ...it, ...patch } : it)))
  }

  function addItemRow() {
    const fallbackSKU = products[0]?.sku ?? ''
    setItems((prev) => [...prev, { sku: fallbackSKU, qty: 1 }])
  }

  function removeItemRow(idx: number) {
    setItems((prev) => prev.filter((_, i) => i !== idx))
  }

  async function handleCreateOrder(e: React.FormEvent) {
    e.preventDefault()
    setFormError(null)
    setSubmitting(true)
    try {
      await api.createOrder({
        customer,
        items: items.map((i) => ({ sku: i.sku, qty: Number(i.qty) })),
      })
      setCreateOpen(false)
      await loadOrders()
    } catch (err) {
      setFormError(err instanceof ApiError ? err.message : 'Failed to create order')
    } finally {
      setSubmitting(false)
    }
  }

  async function openDetail(id: number) {
    setDetail(null)
    setDetailLoading(true)
    try {
      const order = await api.getOrder(id)
      setDetail(order)
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to load order detail')
    } finally {
      setDetailLoading(false)
    }
  }

  async function handleCancel(order: SalesOrder) {
    if (!window.confirm(`Cancel order #${order.id} for ${order.customer}? Stock will be restored.`)) return
    setError(null)
    try {
      const updated = await api.cancelOrder(order.id)
      if (detail?.id === updated.id) {
        setDetail(updated)
      }
      await loadOrders()
    } catch (err) {
      setError(err instanceof ApiError ? err.message : 'Failed to cancel order')
    }
  }

  const estimatedTotal = items.reduce((sum, item) => {
    const prod = products.find((p) => p.sku === item.sku)
    return sum + (prod ? prod.price * Number(item.qty || 0) : 0)
  }, 0)

  return (
    <div>
      {error && <div className="alert alert-error">{error}</div>}

      <div className="toolbar">
        <button className="btn btn-primary" onClick={openCreate} style={{ marginLeft: 'auto' }}>
          + Create Sales Order
        </button>
      </div>

      <div className="table-wrap">
        <table>
          <thead>
            <tr>
              <th>ID</th>
              <th>Customer</th>
              <th>Status</th>
              <th>Total</th>
              <th>Created At</th>
              <th>Actions</th>
            </tr>
          </thead>
          <tbody>
            {loading && orders.length === 0 ? (
              <tr>
                <td colSpan={6}>Loading...</td>
              </tr>
            ) : orders.length === 0 ? (
              <tr>
                <td colSpan={6}>No sales orders yet.</td>
              </tr>
            ) : (
              orders.map((o) => (
                <tr key={o.id}>
                  <td><strong>#{o.id}</strong></td>
                  <td>{o.customer}</td>
                  <td>
                    <span className={`badge ${o.status === 'CREATED' ? 'badge-green' : 'badge-red'}`}>
                      {o.status}
                    </span>
                  </td>
                  <td>{formatIDR(o.total)}</td>
                  <td>{formatDate(o.created_at)}</td>
                  <td>
                    <div className="actions">
                      <button className="btn btn-sm" onClick={() => openDetail(o.id)}>Detail</button>
                      {o.status === 'CREATED' && (
                        <button className="btn btn-sm btn-danger" onClick={() => handleCancel(o)}>
                          Cancel
                        </button>
                      )}
                    </div>
                  </td>
                </tr>
              ))
            )}
          </tbody>
        </table>
      </div>

      {createOpen && (
        <div className="modal-backdrop" onClick={() => setCreateOpen(false)}>
          <div className="modal" onClick={(e) => e.stopPropagation()}>
            <h2>Create Sales Order</h2>
            {formError && <div className="alert alert-error">{formError}</div>}
            <form onSubmit={handleCreateOrder}>
              <div className="form-group">
                <label>Customer Name</label>
                <input
                  className="input"
                  required
                  placeholder="e.g. PT Maju Jaya"
                  value={customer}
                  onChange={(e) => setCustomer(e.target.value)}
                />
              </div>

              <div className="form-group">
                <label>Items</label>
                {items.map((item, idx) => (
                  <div key={idx} style={{ display: 'flex', gap: 8, marginBottom: 8 }}>
                    <select
                      className="select"
                      style={{ flex: 1 }}
                      value={item.sku}
                      onChange={(e) => updateItem(idx, { sku: e.target.value })}
                    >
                      {products.map((p) => (
                        <option key={p.sku} value={p.sku}>
                          {p.sku} — {p.name} (stock: {p.stock})
                        </option>
                      ))}
                    </select>
                    <input
                      className="input"
                      type="number"
                      min={1}
                      style={{ width: 80, minWidth: 80 }}
                      value={item.qty}
                      onChange={(e) => updateItem(idx, { qty: Number(e.target.value) })}
                    />
                    {items.length > 1 && (
                      <button type="button" className="btn btn-sm btn-danger" onClick={() => removeItemRow(idx)}>
                        ×
                      </button>
                    )}
                  </div>
                ))}
                <button type="button" className="btn btn-sm" onClick={addItemRow} style={{ alignSelf: 'flex-start' }}>
                  + Add Item
                </button>
              </div>

              <p style={{ fontWeight: 600, marginTop: 8 }}>
                Estimated Total: {formatIDR(estimatedTotal)}
              </p>

              <div className="modal-footer">
                <button type="button" className="btn" onClick={() => setCreateOpen(false)}>Cancel</button>
                <button type="submit" className="btn btn-primary" disabled={submitting}>
                  {submitting ? 'Creating...' : 'Create Order'}
                </button>
              </div>
            </form>
          </div>
        </div>
      )}

      {(detailLoading || detail) && (
        <div className="modal-backdrop" onClick={() => setDetail(null)}>
          <div className="modal" onClick={(e) => e.stopPropagation()}>
            {detailLoading || !detail ? (
              <p>Loading...</p>
            ) : (
              <>
                <h2>Order #{detail.id}</h2>
                <p style={{ margin: '4px 0' }}>Customer: <strong>{detail.customer}</strong></p>
                <p style={{ margin: '4px 0' }}>
                  Status:{' '}
                  <span className={`badge ${detail.status === 'CREATED' ? 'badge-green' : 'badge-red'}`}>
                    {detail.status}
                  </span>
                </p>
                <p style={{ margin: '4px 0 12px', color: '#6b7280', fontSize: '0.85rem' }}>
                  Created: {formatDate(detail.created_at)}
                  {detail.cancelled_at && ` | Cancelled: ${formatDate(detail.cancelled_at)}`}
                </p>
                <table>
                  <thead>
                    <tr>
                      <th>SKU</th>
                      <th>Qty</th>
                      <th>Unit Price</th>
                      <th>Subtotal</th>
                    </tr>
                  </thead>
                  <tbody>
                    {detail.items.map((it) => (
                      <tr key={it.id}>
                        <td>{it.sku}</td>
                        <td>{it.qty}</td>
                        <td>{formatIDR(it.unit_price)}</td>
                        <td>{formatIDR(it.unit_price * it.qty)}</td>
                      </tr>
                    ))}
                  </tbody>
                </table>
                <p style={{ textAlign: 'right', fontWeight: 700, marginTop: 12 }}>
                  Total: {formatIDR(detail.total)}
                </p>
                <div className="modal-footer">
                  {detail.status === 'CREATED' && (
                    <button className="btn btn-danger" onClick={() => handleCancel(detail)}>
                      Cancel Order
                    </button>
                  )}
                  <button className="btn" onClick={() => setDetail(null)}>Close</button>
                </div>
              </>
            )}
          </div>
        </div>
      )}
    </div>
  )
}