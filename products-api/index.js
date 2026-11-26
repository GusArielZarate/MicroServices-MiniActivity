const fs = require("fs");
const path = require("path");
const express = require("express");
const Database = require("better-sqlite3");

const PORT = Number(process.env.PORT || 8003);
const DB_PATH = process.env.DB_PATH || "/app/data/products.db";

fs.mkdirSync(path.dirname(DB_PATH), { recursive: true });

const db = new Database(DB_PATH);
db.pragma("busy_timeout = 5000");

db.exec(`
  CREATE TABLE IF NOT EXISTS products (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL,
    price REAL NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    stock INTEGER NOT NULL DEFAULT 0,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
  );
`);

const app = express();
app.use(express.json());

app.use((err, _req, res, next) => {
  if (err instanceof SyntaxError && err.status === 400 && "body" in err) {
    return res.status(400).json({ detail: "JSON inválido en el cuerpo de la solicitud." });
  }
  return next(err);
});

function parseId(rawId) {
  const id = Number(rawId);
  if (!Number.isInteger(id) || id <= 0) {
    return null;
  }
  return id;
}

app.get("/health", (_req, res) => {
  db.prepare("SELECT 1").get();
  res.json({
    status: "ok",
    service: "products-api",
    database: path.basename(DB_PATH),
  });
});

app.post("/products", (req, res) => {
  const { name, price, description = "", stock = 0 } = req.body || {};

  if (typeof name !== "string" || name.trim() === "") {
    return res.status(400).json({ detail: "El campo 'name' es obligatorio." });
  }
  if (typeof price !== "number" || Number.isNaN(price) || price < 0) {
    return res.status(400).json({ detail: "El campo 'price' debe ser un número mayor o igual a 0." });
  }
  if (typeof description !== "string") {
    return res.status(400).json({ detail: "El campo 'description' debe ser texto." });
  }
  if (!Number.isInteger(stock) || stock < 0) {
    return res.status(400).json({ detail: "El campo 'stock' debe ser un entero mayor o igual a 0." });
  }

  const now = new Date().toISOString();
  const info = db
    .prepare(
      `INSERT INTO products (name, price, description, stock, created_at, updated_at)
       VALUES (?, ?, ?, ?, ?, ?)`
    )
    .run(name.trim(), price, description.trim(), stock, now, now);

  const product = db.prepare("SELECT * FROM products WHERE id = ?").get(info.lastInsertRowid);
  return res.status(201).json({
    message: "Producto registrado exitosamente",
    product,
  });
});

app.get("/products", (_req, res) => {
  const products = db.prepare("SELECT * FROM products ORDER BY id ASC").all();
  res.json({
    products,
    total: products.length,
  });
});

app.get("/products/:id", (req, res) => {
  const id = parseId(req.params.id);
  if (id === null) {
    return res.status(400).json({ detail: "ID de producto inválido." });
  }

  const product = db.prepare("SELECT * FROM products WHERE id = ?").get(id);
  if (!product) {
    return res.status(404).json({ detail: "Producto no encontrado." });
  }

  return res.json({ product });
});

app.put("/products/:id", (req, res) => {
  const id = parseId(req.params.id);
  if (id === null) {
    return res.status(400).json({ detail: "ID de producto inválido." });
  }

  const existing = db.prepare("SELECT * FROM products WHERE id = ?").get(id);
  if (!existing) {
    return res.status(404).json({ detail: "Producto no encontrado." });
  }

  const { name, price, description, stock } = req.body || {};

  let nextName = existing.name;
  if (name !== undefined) {
    if (typeof name !== "string" || name.trim() === "") {
      return res.status(400).json({ detail: "El campo 'name' no puede estar vacío." });
    }
    nextName = name.trim();
  }

  let nextPrice = existing.price;
  if (price !== undefined) {
    if (typeof price !== "number" || Number.isNaN(price) || price < 0) {
      return res.status(400).json({ detail: "El campo 'price' debe ser un número mayor o igual a 0." });
    }
    nextPrice = price;
  }

  let nextDescription = existing.description;
  if (description !== undefined) {
    if (typeof description !== "string") {
      return res.status(400).json({ detail: "El campo 'description' debe ser texto." });
    }
    nextDescription = description.trim();
  }

  let nextStock = existing.stock;
  if (stock !== undefined) {
    if (!Number.isInteger(stock) || stock < 0) {
      return res.status(400).json({ detail: "El campo 'stock' debe ser un entero mayor o igual a 0." });
    }
    nextStock = stock;
  }

  const now = new Date().toISOString();
  db.prepare(
    `UPDATE products
     SET name = ?, price = ?, description = ?, stock = ?, updated_at = ?
     WHERE id = ?`
  ).run(nextName, nextPrice, nextDescription, nextStock, now, id);

  const updated = db.prepare("SELECT * FROM products WHERE id = ?").get(id);
  return res.json({
    message: "Producto actualizado exitosamente",
    product: updated,
  });
});

app.delete("/products/:id", (req, res) => {
  const id = parseId(req.params.id);
  if (id === null) {
    return res.status(400).json({ detail: "ID de producto inválido." });
  }

  const result = db.prepare("DELETE FROM products WHERE id = ?").run(id);
  if (result.changes === 0) {
    return res.status(404).json({ detail: "Producto no encontrado." });
  }

  return res.json({
    message: "Producto eliminado exitosamente",
    deleted_id: id,
  });
});

app.listen(PORT, "0.0.0.0", () => {
  console.log(`Products API listening on port ${PORT} using ${DB_PATH}`);
});
