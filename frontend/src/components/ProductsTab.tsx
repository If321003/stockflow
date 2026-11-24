import { useCallback, useEffect, useState } from 'react'
import { api, ApiError, type Product, type ProductSummary, type StockMovement } from '../api'

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

export function ProductsTab() {
  const [products, setProducts] = useState<Product[]>([])
  const [summary, setSummary] = useState<ProductSummary | null>(null)
  const [search, setSearch] = useState('')
  const [filter, setFilter] = useState('all')
  const [page, setPage] = useState(1)
  const [total, setTotal] = useState(0)
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)

  // Form modal (create / edit)
  const [formOpen, setFormOpen] = useState(false)
  const [editing, setEditing] = useState<Product | null>(null)
  const [sku, setSku] = useState('')
  const [name, setName] = useState('')
  const [price, setPrice] = useState('0')
  const [stock, setStock] = useState('0')
  const [isActive, setIsActive] = useState(true)
  const [formError, setFormError] = useState<string | null>(null)

  // Stock adjustment modal
  const [adjustTarget, setAdjustTarget] = useState<Product | null>(null)
  const [adjType, setAdjType] = useState<'in' | 'out'>('in')
  const [adjQty, setAdjQty] = useState('1')
  const [adjNote, setAdjNote] = useState('')
  const [adjError, setAdjError] = useState<string | null>(null)

  // Movement history modal
  const [historyTarget, setHistoryTarget] = useState<Product | null>(null)
  const [movements, setMovements] = useState<StockMovement[]>([])
  const [historyLoading, setHistoryLoading] = useState(false)

  const limit = 10

  const loadData = useCallback(async () => {
    setLoading(true)
    setError(null)
    try {
      const [listRes, sumRes] = await Promise.all([
        api.listProducts({ search, filter, page, limit }),
        api.getSummary(),
      ])
      setProducts(listRes.data)
      setTotal(listRes.total)
      setSummary(sumRes)
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to load products')
    } finally {
      setLoading(false)
    }
  }, [search, filter, page])

  useEffect(() => {
    const timer = setTimeout(() => {
      void loadData()
    }, 200)
    return () => clearTimeout(timer)
  }, [loadData])

  function openCreate() {
    setEditing(null)
    setSku('')
    setName('')
    setPrice('0')
    setStock('0')
    setIsActive(true)
    setFormError(null)
    setFormOpen(true)
  }

  function openEdit(p: Product) {
    setEditing(p)
    setSku(p.sku)
    setName(p.name)
    setPrice(String(p.price))
    setStock(String(p.stock))
    setIsActive(p.is_active)
    setFormError(null)
    setFormOpen(true)
  }

  async function handleSaveProduct(e: React.FormEvent) {
    e.preventDefault()
    setFormError(null)
    try {
      if (editing) {
        await api.updateProduct(editing.id, {
          sku,
          name,
          price: Number(price),
          is_active: isActive,
        })
      } else {
        await api.createProduct({
          sku,
          name,
          price: Number(price),
          stock: Number(stock),
          is_active: isActive,
        })
      }
      setFormOpen(false)
      await loadData()
    } catch (err) {
      setFormError(err instanceof ApiError ? err.message : 'Failed to save product')
    }
  }

  async function handleDelete(p: Product) {
    if (!window.confirm(`Delete product ${p.sku} (${p.name})?`)) return
    setError(null)
    try {
      await api.deleteProduct(p.id)
      await loadData()
    } catch (err) {
      setError(err instanceof ApiError ? err.message : 'Failed to delete product')
    }
  }

  function openAdjust(p: Product) {
    setAdjustTarget(p)
    setAdjType('in')
    setAdjQty('1')
    setAdjNote('')
    setAdjError(null)
  }

  async function handleAdjustStock(e: React.FormEvent) {
    e.preventDefault()
    if (!adjustTarget) return
    setAdjError(null)
    try {
      await api.adjustStock(adjustTarget.id, {
        type: adjType,
        qty: Number(adjQty),
        note: adjNote,
      })
      setAdjustTarget(null)
      await loadData()
    } catch (err) {
      setAdjError(err instanceof ApiError ? err.message : 'Failed to adjust stock')
    }
  }

  async function openHistory(p: Product) {
    setHistoryTarget(p)
    setMovements([])
    setHistoryLoading(true)
    try {
      const res = await api.listMovements(p.id)
      setMovements(res.data)
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to load movements')
    } finally {
      setHistoryLoading(false)
    }
  }

  const totalPages = Math.max(1, Math.ceil(total / limit))

  return (
    <div>
      {summary && (
        <div className="summary-grid">
          <div className="card">
            <div className="card-label">Total Products</div>
            <div className="card-value">{summary.total_products}</div>
          </div>
          <div className="card">
            <div className="card-label">Active Products</div>
            <div className="card-value">{summary.active_products}</div>
          </div>
          <div className="card">
            <div className="card-label">Total Stock Value</div>
            <div className="card-value">{formatIDR(summary.total_stock_value)}</div>
          </div>
          <div className="card">
            <div className="card-label">Low Stock (&le; 10)</div>
            <div className="card-value">{summary.low_stock_count}</div>
          </div>
        </div>
      )}

      {error && <div className="alert alert-error">{error}</div>}

      <div className="toolbar">
        <input
          className="input"
          placeholder="Search SKU or name..."
          value={search}
          onChange={(e) => {
            setSearch(e.target.value)
            setPage(1)
          }}
        />
        <select
          className="select"
          value={filter}
          onChange={(e) => {
            setFilter(e.target.value)
            setPage(1)
          }}
        >
          <option value="all">All Products</option>
          <option value="active">Active</option>
          <option value="inactive">Inactive</option>
          <option value="low_stock">Low Stock (&le; 10)</option>
        </select>
        <button className="btn btn-primary" onClick={openCreate} style={{ marginLeft: 'auto' }}>
          + Add Product
        </button>
      </div>

      <div className="table-wrap">
        <table>
          <thead>
            <tr>
              <th>SKU</th>
              <th>Name</th>
              <th>Price</th>
              <th>Stock</th>
              <th>Status</th>
              <th>Last Movement</th>
              <th>Actions</th>
            </tr>
          </thead>
          <tbody>
            {loading && products.length === 0 ? (
              <tr>
                <td colSpan={7}>Loading...</td>
              </tr>
            ) : products.length === 0 ? (
              <tr>
                <td colSpan={7}>No products found.</td>
              </tr>
            ) : (
              products.map((p) => (
                <tr key={p.id}>
                  <td><strong>{p.sku}</strong></td>
                  <td>{p.name}</td>
                  <td>{formatIDR(p.price)}</td>
                  <td>
                    <span className={`badge ${p.stock <= 10 ? 'badge-yellow' : 'badge-green'}`}>
                      {p.stock}
                    </span>
                  </td>
                  <td>
                    <span className={`badge ${p.is_active ? 'badge-green' : 'badge-gray'}`}>
                      {p.is_active ? 'Active' : 'Inactive'}
                    </span>
                  </td>
                  <td>{formatDate(p.last_movement_at)}</td>
                  <td>
                    <div className="actions">
                      <button className="btn btn-sm" onClick={() => openAdjust(p)}>Stock</button>
                      <button className="btn btn-sm" onClick={() => openHistory(p)}>History</button>
                      <button className="btn btn-sm" onClick={() => openEdit(p)}>Edit</button>
                      <button className="btn btn-sm btn-danger" onClick={() => handleDelete(p)}>Delete</button>
                    </div>
                  </td>
                </tr>
              ))
            )}
          </tbody>
        </table>
      </div>

      <div className="pagination">
        <span>
          Page {page} of {totalPages} ({total} items)
        </span>
        <div className="actions">
          <button className="btn btn-sm" disabled={page <= 1} onClick={() => setPage((p) => p - 1)}>
            Prev
          </button>
          <button className="btn btn-sm" disabled={page >= totalPages} onClick={() => setPage((p) => p + 1)}>
            Next
          </button>
        </div>
      </div>

      {formOpen && (
        <div className="modal-backdrop" onClick={() => setFormOpen(false)}>
          <div className="modal" onClick={(e) => e.stopPropagation()}>
            <h2>{editing ? `Edit ${editing.sku}` : 'Add Product'}</h2>
            {formError && <div className="alert alert-error">{formError}</div>}
            <form onSubmit={handleSaveProduct}>
              <div className="form-group">
                <label>SKU</label>
                <input className="input" required value={sku} onChange={(e) => setSku(e.target.value)} />
              </div>
              <div className="form-group">
                <label>Name</label>
                <input className="input" required value={name} onChange={(e) => setName(e.target.value)} />
              </div>
              <div className="form-group">
                <label>Price (IDR)</label>
                <input
                  className="input"
                  type="number"
                  min={0}
                  required
                  value={price}
                  onChange={(e) => setPrice(e.target.value)}
                />
              </div>
              {!editing && (
                <div className="form-group">
                  <label>Initial Stock</label>
                  <input
                    className="input"
                    type="number"
                    min={0}
                    required
                    value={stock}
                    onChange={(e) => setStock(e.target.value)}
                  />
                </div>
              )}
              <div className="form-group">
                <label>
                  <input
                    type="checkbox"
                    checked={isActive}
                    onChange={(e) => setIsActive(e.target.checked)}
                  />{' '}
                  Active
                </label>
              </div>
              <div className="modal-footer">
                <button type="button" className="btn" onClick={() => setFormOpen(false)}>Cancel</button>
                <button type="submit" className="btn btn-primary">Save</button>
              </div>
            </form>
          </div>
        </div>
      )}

      {adjustTarget && (
        <div className="modal-backdrop" onClick={() => setAdjustTarget(null)}>
          <div className="modal" onClick={(e) => e.stopPropagation()}>
            <h2>Adjust Stock — {adjustTarget.sku}</h2>
            <p style={{ marginTop: 0, color: '#6b7280', fontSize: '0.9rem' }}>
              Current stock: <strong>{adjustTarget.stock}</strong>
            </p>
            {adjError && <div className="alert alert-error">{adjError}</div>}
            <form onSubmit={handleAdjustStock}>
              <div className="form-group">
                <label>Type</label>
                <select
                  className="select"
                  value={adjType}
                  onChange={(e) => setAdjType(e.target.value as 'in' | 'out')}
                >
                  <option value="in">Stock In (+)</option>
                  <option value="out">Stock Out (-)</option>
                </select>
              </div>
              <div className="form-group">
                <label>Quantity</label>
                <input
                  className="input"
                  type="number"
                  min={1}
                  required
                  value={adjQty}
                  onChange={(e) => setAdjQty(e.target.value)}
                />
              </div>
              <div className="form-group">
                <label>Note</label>
                <input
                  className="input"
                  placeholder="Optional reason..."
                  value={adjNote}
                  onChange={(e) => setAdjNote(e.target.value)}
                />
              </div>
              <div className="modal-footer">
                <button type="button" className="btn" onClick={() => setAdjustTarget(null)}>Cancel</button>
                <button type="submit" className="btn btn-primary">Submit</button>
              </div>
            </form>
          </div>
        </div>
      )}

      {historyTarget && (
        <div className="modal-backdrop" onClick={() => setHistoryTarget(null)}>
          <div className="modal" onClick={(e) => e.stopPropagation()}>
            <h2>Stock Movements — {historyTarget.sku}</h2>
            {historyLoading ? (
              <p>Loading...</p>
            ) : movements.length === 0 ? (
              <p>No movement history.</p>
            ) : (
              <table>
                <thead>
                  <tr>
                    <th>Type</th>
                    <th>Qty</th>
                    <th>Note</th>
                    <th>Time</th>
                  </tr>
                </thead>
                <tbody>
                  {movements.map((m) => (
                    <tr key={m.id}>
                      <td>
                        <span className={`badge ${m.type === 'in' ? 'badge-green' : 'badge-red'}`}>
                          {m.type.toUpperCase()}
                        </span>
                      </td>
                      <td>{m.qty}</td>
                      <td>{m.note || '-'}</td>
                      <td>{formatDate(m.created_at)}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            )}
            <div className="modal-footer">
              <button className="btn" onClick={() => setHistoryTarget(null)}>Close</button>
            </div>
          </div>
        </div>
      )}
    </div>
  )
}