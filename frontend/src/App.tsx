import { useState } from 'react'
import { ProductsTab } from './components/ProductsTab'
import { OrdersTab } from './components/OrdersTab'

export default function App() {
  const [tab, setTab] = useState<'products' | 'orders'>('products')

  return (
    <div className="app">
      <header className="header">
        <h1>StockFlow Inventory</h1>
        <div className="tabs">
          <button
            className={`tab-btn ${tab === 'products' ? 'active' : ''}`}
            onClick={() => setTab('products')}
          >
            Products
          </button>
          <button
            className={`tab-btn ${tab === 'orders' ? 'active' : ''}`}
            onClick={() => setTab('orders')}
          >
            Sales Orders
          </button>
        </div>
      </header>

      {tab === 'products' ? <ProductsTab /> : <OrdersTab />}
    </div>
  )
}