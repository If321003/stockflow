StockFlow Inventory

Aplikasi manajemen inventori, mutasi stok, dan penjualan (sales order) yang dibangun dengan Go, PostgreSQL, dan React (TypeScript + Vite). Operasi yang mengubah stok dan status pesanan dijalankan di dalam transaksi database dengan penguncian baris (FOR UPDATE) serta validasi kondisi stok agar tetap konsisten saat diakses secara bersamaan.

Tech Stack

Backend: Go (net/http, pgx/v5)

Database: PostgreSQL 16 (via Docker Compose)

Frontend: React, TypeScript, Vite

Struktur Proyek

stockflow/
├── backend/
│   ├── cmd/
│   │   └── server/
│   │       └── main.go
│   ├── internal/
│   │   ├── handler/
│   │   │   ├── handler.go
│   │   │   ├── handler_test.go
│   │   │   ├── orders.go
│   │   │   └── products.go
│   │   ├── model/
│   │   │   └── model.go
│   │   └── store/
│   │       ├── orders.go
│   │       ├── products.go
│   │       ├── store.go
│   │       └── store_test.go
│   ├── migrations/
│   │   ├── 001_init.sql
│   │   └── 002_seed.sql
│   ├── go.mod
│   └── go.sum
├── frontend/
│   ├── src/
│   │   ├── components/
│   │   │   ├── OrdersTab.tsx
│   │   │   └── ProductsTab.tsx
│   │   ├── api.ts
│   │   ├── App.tsx
│   │   └── index.css
│   ├── index.html
│   ├── package.json
│   └── vite.config.ts
├── .gitignore
├── docker-compose.yml
└── README.md


Menjalankan Proyek Secara Lokal

1. Jalankan Database PostgreSQL

Pastikan Docker Desktop sudah aktif, lalu dari folder root stockflow:

docker compose up -d


File migrasi dan data awal di backend/migrations/001_init.sql serta backend/migrations/002_seed.sql akan dijalankan otomatis saat volume database dibuat pertama kali.

2. Jalankan Backend API

cd backend
go run ./cmd/server


Server berjalan di http://localhost:8080. Secara default aplikasi menggunakan:

DATABASE_URL: postgres://stockflow:stockflow_pass@localhost:5432/stockflow?sslmode=disable

PORT: 8080

3. Jalankan Frontend

Buka terminal baru:

cd frontend
npm install
npm run dev


Aplikasi web dapat diakses di http://localhost:5173. Request ke /api diteruskan ke backend melalui proxy di frontend/vite.config.ts.

Menjalankan Backend Test

Pastikan kontainer PostgreSQL sedang berjalan, lalu dari folder backend:

go test -v ./...


Pengujian mencakup penyesuaian stok, proteksi penghapusan produk yang sudah memiliki riwayat mutasi, rollback saat stok tidak mencukupi, pembatalan pesanan beserta pengembalian stok, skenario pembuatan pesanan secara paralel, serta alur respons endpoint HTTP.

Endpoint API

Produk & Stok

GET /api/products — Mengambil daftar produk (search, filter berupa all, active, inactive, atau low_stock, page, dan limit)

GET /api/products/summary — Mengambil ringkasan total produk, produk aktif, total nilai stok, dan jumlah produk dengan stok <= 10

POST /api/products — Membuat produk baru

PUT /api/products/{id} — Memperbarui informasi produk

DELETE /api/products/{id} — Menghapus produk jika belum memiliki riwayat mutasi stok

POST /api/products/{id}/stock — Mencatat mutasi stok masuk (in) atau keluar (out) secara transaksional

GET /api/products/{id}/movements — Melihat riwayat mutasi stok berdasarkan produk

Sales Order

GET /api/orders — Mengambil daftar sales order

POST /api/orders — Membuat sales order baru, mengurangi stok secara atomik, dan mencatat mutasi stok keluar (out)

GET /api/orders/{id} — Melihat detail sales order beserta daftar item-nya

POST /api/orders/{id}/cancel — Membatalkan sales order, mengembalikan stok, dan mencatat mutasi stok masuk (in)