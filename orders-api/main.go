package main

import (
	"database/sql"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

type Order struct {
	ID           int64   `json:"id"`
	CustomerName string  `json:"customer_name"`
	ProductName  string  `json:"product_name"`
	Quantity     int     `json:"quantity"`
	Total        float64 `json:"total"`
	Status       string  `json:"status"`
	OrderDate    string  `json:"order_date"`
	UpdatedAt    string  `json:"updated_at"`
}

type CreateOrderRequest struct {
	CustomerName string  `json:"customer_name"`
	ProductName  string  `json:"product_name"`
	Quantity     int     `json:"quantity"`
	Total        float64 `json:"total"`
	Status       string  `json:"status"`
}

type UpdateOrderRequest struct {
	Status       *string  `json:"status"`
	CustomerName *string  `json:"customer_name"`
	ProductName  *string  `json:"product_name"`
	Quantity     *int     `json:"quantity"`
	Total        *float64 `json:"total"`
}

var validStatuses = map[string]bool{
	"pending":    true,
	"processing": true,
	"shipped":    true,
	"delivered":  true,
	"cancelled":  true,
}

func main() {
	dbPath := getEnv("DB_PATH", "/app/data/orders.db")
	port := getEnv("PORT", "8002")

	if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
		log.Fatalf("error creating database directory: %v", err)
	}

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		log.Fatalf("error opening sqlite database: %v", err)
	}
	defer db.Close()

	if _, err := db.Exec("PRAGMA busy_timeout = 5000;"); err != nil {
		log.Fatalf("error configuring sqlite: %v", err)
	}

	if err := initDB(db); err != nil {
		log.Fatalf("error initializing schema: %v", err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", handleHealth(db, filepath.Base(dbPath)))
	mux.HandleFunc("POST /orders", handleCreateOrder(db))
	mux.HandleFunc("GET /orders", handleListOrders(db))
	mux.HandleFunc("GET /orders/{id}", handleGetOrder(db))
	mux.HandleFunc("PUT /orders/{id}", handleUpdateOrder(db))
	mux.HandleFunc("PATCH /orders/{id}", handleUpdateOrder(db))
	mux.HandleFunc("PATCH /orders/{id}/status", handleUpdateOrder(db))
	mux.HandleFunc("DELETE /orders/{id}", handleDeleteOrder(db))

	addr := ":" + port
	log.Printf("Orders API listening on %s using %s", addr, dbPath)
	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatalf("server error: %v", err)
	}
}

func getEnv(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}

func initDB(db *sql.DB) error {
	query := `
	CREATE TABLE IF NOT EXISTS orders (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		customer_name TEXT NOT NULL,
		product_name TEXT NOT NULL,
		quantity INTEGER NOT NULL,
		total REAL NOT NULL,
		status TEXT NOT NULL,
		order_date TEXT NOT NULL,
		updated_at TEXT NOT NULL
	);`
	_, err := db.Exec(query)
	return err
}

func writeJSON(w http.ResponseWriter, statusCode int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(statusCode)
	_ = json.NewEncoder(w).Encode(payload)
}

func parseID(r *http.Request) (int64, error) {
	idStr := r.PathValue("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || id <= 0 {
		return 0, errors.New("invalid order id")
	}
	return id, nil
}

func handleHealth(db *sql.DB, dbName string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := db.Ping(); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{
				"status":   "error",
				"service":  "orders-api",
				"database": dbName,
			})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"status":   "ok",
			"service":  "orders-api",
			"database": dbName,
		})
	}
}

func handleCreateOrder(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req CreateOrderRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"detail": "JSON inválido"})
			return
		}

		req.CustomerName = strings.TrimSpace(req.CustomerName)
		req.ProductName = strings.TrimSpace(req.ProductName)
		req.Status = strings.ToLower(strings.TrimSpace(req.Status))
		if req.Status == "" {
			req.Status = "pending"
		}

		if req.CustomerName == "" || req.ProductName == "" || req.Quantity <= 0 || req.Total < 0 {
			writeJSON(w, http.StatusBadRequest, map[string]string{
				"detail": "Campos inválidos: customer_name, product_name, quantity (>0) y total (>=0) son requeridos.",
			})
			return
		}
		if !validStatuses[req.Status] {
			writeJSON(w, http.StatusBadRequest, map[string]string{
				"detail": "Estado inválido. Valores permitidos: pending, processing, shipped, delivered, cancelled.",
			})
			return
		}

		now := time.Now().UTC().Format(time.RFC3339)
		res, err := db.Exec(
			`INSERT INTO orders (customer_name, product_name, quantity, total, status, order_date, updated_at)
			 VALUES (?, ?, ?, ?, ?, ?, ?)`,
			req.CustomerName, req.ProductName, req.Quantity, req.Total, req.Status, now, now,
		)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"detail": "Error al crear el pedido"})
			return
		}

		id, _ := res.LastInsertId()
		order, err := fetchOrderByID(db, id)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"detail": "Error al leer el pedido creado"})
			return
		}

		writeJSON(w, http.StatusCreated, map[string]any{
			"message": "Pedido creado exitosamente",
			"order":   order,
		})
	}
}

func handleListOrders(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		rows, err := db.Query(
			`SELECT id, customer_name, product_name, quantity, total, status, order_date, updated_at
			 FROM orders ORDER BY id ASC`,
		)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"detail": "Error al consultar pedidos"})
			return
		}
		defer rows.Close()

		orders := make([]Order, 0)
		for rows.Next() {
			var o Order
			if err := rows.Scan(&o.ID, &o.CustomerName, &o.ProductName, &o.Quantity, &o.Total, &o.Status, &o.OrderDate, &o.UpdatedAt); err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]string{"detail": "Error al procesar pedidos"})
				return
			}
			orders = append(orders, o)
		}

		writeJSON(w, http.StatusOK, map[string]any{
			"orders": orders,
			"total":  len(orders),
		})
	}
}

func handleGetOrder(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := parseID(r)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"detail": "ID de pedido inválido"})
			return
		}

		order, err := fetchOrderByID(db, id)
		if errors.Is(err, sql.ErrNoRows) {
			writeJSON(w, http.StatusNotFound, map[string]string{"detail": "Pedido no encontrado"})
			return
		}
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"detail": "Error al consultar el pedido"})
			return
		}

		writeJSON(w, http.StatusOK, map[string]any{"order": order})
	}
}

func handleUpdateOrder(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := parseID(r)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"detail": "ID de pedido inválido"})
			return
		}

		existing, err := fetchOrderByID(db, id)
		if errors.Is(err, sql.ErrNoRows) {
			writeJSON(w, http.StatusNotFound, map[string]string{"detail": "Pedido no encontrado"})
			return
		}
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"detail": "Error al consultar el pedido"})
			return
		}

		var req UpdateOrderRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"detail": "JSON inválido"})
			return
		}

		if req.CustomerName != nil {
			trimmed := strings.TrimSpace(*req.CustomerName)
			if trimmed == "" {
				writeJSON(w, http.StatusBadRequest, map[string]string{"detail": "customer_name no puede estar vacío"})
				return
			}
			existing.CustomerName = trimmed
		}
		if req.ProductName != nil {
			trimmed := strings.TrimSpace(*req.ProductName)
			if trimmed == "" {
				writeJSON(w, http.StatusBadRequest, map[string]string{"detail": "product_name no puede estar vacío"})
				return
			}
			existing.ProductName = trimmed
		}
		if req.Quantity != nil {
			if *req.Quantity <= 0 {
				writeJSON(w, http.StatusBadRequest, map[string]string{"detail": "quantity debe ser mayor a 0"})
				return
			}
			existing.Quantity = *req.Quantity
		}
		if req.Total != nil {
			if *req.Total < 0 {
				writeJSON(w, http.StatusBadRequest, map[string]string{"detail": "total no puede ser negativo"})
				return
			}
			existing.Total = *req.Total
		}
		if req.Status != nil {
			statusVal := strings.ToLower(strings.TrimSpace(*req.Status))
			if !validStatuses[statusVal] {
				writeJSON(w, http.StatusBadRequest, map[string]string{
					"detail": "Estado inválido. Valores permitidos: pending, processing, shipped, delivered, cancelled.",
				})
				return
			}
			existing.Status = statusVal
		}

		now := time.Now().UTC().Format(time.RFC3339)
		_, err = db.Exec(
			`UPDATE orders
			 SET customer_name = ?, product_name = ?, quantity = ?, total = ?, status = ?, updated_at = ?
			 WHERE id = ?`,
			existing.CustomerName, existing.ProductName, existing.Quantity, existing.Total, existing.Status, now, id,
		)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"detail": "Error al actualizar el pedido"})
			return
		}

		updated, err := fetchOrderByID(db, id)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"detail": "Error al recuperar pedido actualizado"})
			return
		}

		writeJSON(w, http.StatusOK, map[string]any{
			"message": "Pedido actualizado exitosamente",
			"order":   updated,
		})
	}
}

func handleDeleteOrder(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := parseID(r)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"detail": "ID de pedido inválido"})
			return
		}

		res, err := db.Exec(`DELETE FROM orders WHERE id = ?`, id)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"detail": "Error al eliminar el pedido"})
			return
		}
		affected, _ := res.RowsAffected()
		if affected == 0 {
			writeJSON(w, http.StatusNotFound, map[string]string{"detail": "Pedido no encontrado"})
			return
		}

		writeJSON(w, http.StatusOK, map[string]any{
			"message":    "Pedido eliminado exitosamente",
			"deleted_id": id,
		})
	}
}

func fetchOrderByID(db *sql.DB, id int64) (Order, error) {
	var o Order
	err := db.QueryRow(
		`SELECT id, customer_name, product_name, quantity, total, status, order_date, updated_at
		 FROM orders WHERE id = ?`,
		id,
	).Scan(&o.ID, &o.CustomerName, &o.ProductName, &o.Quantity, &o.Total, &o.Status, &o.OrderDate, &o.UpdatedAt)
	return o, err
}
