INSERT INTO products (sku, name, price, stock, is_active) VALUES
('KBD-001', 'Mechanical Keyboard 75%', 650000, 24, TRUE),
('MSE-002', 'Wireless Ergonomic Mouse', 320000, 18, TRUE),
('MON-003', 'IPS Monitor 24 Inch 100Hz', 1750000, 7, TRUE),
('HUB-004', 'USB-C Multiport Hub 7-in-1', 285000, 35, TRUE),
('SSD-005', 'NVMe SSD 1TB Gen4', 1290000, 9, TRUE),
('RAM-006', 'DDR4 16GB 3200MHz SODIMM', 540000, 14, TRUE),
('CAM-007', 'Full HD Webcam 1080p', 410000, 5, TRUE),
('HDS-008', 'Noise Cancelling Headset', 495000, 12, TRUE),
('STD-009', 'Aluminum Laptop Stand', 195000, 40, TRUE),
('CBL-010', 'HDMI 2.1 Braided Cable 2m', 95000, 60, TRUE),
('CHG-011', 'GaN Charger 65W Dual Port', 310000, 8, TRUE),
('UPS-012', 'UPS Backup 650VA', 780000, 3, TRUE),
('PRT-013', 'Thermal Label Printer (Discontinued)', 890000, 0, FALSE),
('BAG-014', 'Waterproof Laptop Sleeve 14"', 165000, 15, TRUE);

INSERT INTO stock_movements (product_id, type, qty, note, created_at) VALUES
(1, 'in', 26, 'Initial stock', now() - interval '5 days'),
(2, 'in', 20, 'Initial stock', now() - interval '5 days'),
(3, 'in', 8, 'Initial stock', now() - interval '4 days'),
(4, 'in', 35, 'Initial stock', now() - interval '4 days'),
(5, 'in', 10, 'Restock batch 12', now() - interval '3 days'),
(6, 'in', 14, 'Restock batch 12', now() - interval '3 days'),
(7, 'in', 5, 'Initial stock', now() - interval '2 days'),
(8, 'in', 12, 'Initial stock', now() - interval '2 days'),
(9, 'in', 40, 'Initial stock', now() - interval '2 days'),
(10, 'in', 60, 'Initial stock', now() - interval '1 day'),
(11, 'in', 8, 'Initial stock', now() - interval '12 hours'),
(12, 'in', 3, 'Initial stock', now() - interval '6 hours');

INSERT INTO sales_orders (customer, status, total, created_at) VALUES
('PT Maju Bersama', 'CREATED', 1940000, now() - interval '1 day'),
('CV Karya Mandiri', 'CREATED', 3040000, now() - interval '3 hours');

INSERT INTO sales_order_items (order_id, sku, qty, unit_price) VALUES
(1, 'KBD-001', 2, 650000),
(1, 'MSE-002', 2, 320000),
(2, 'MON-003', 1, 1750000),
(2, 'SSD-005', 1, 1290000);

INSERT INTO stock_movements (product_id, type, qty, note, created_at) VALUES
(1, 'out', 2, 'Sales order #1', now() - interval '1 day'),
(2, 'out', 2, 'Sales order #1', now() - interval '1 day'),
(3, 'out', 1, 'Sales order #2', now() - interval '3 hours'),
(5, 'out', 1, 'Sales order #2', now() - interval '3 hours');