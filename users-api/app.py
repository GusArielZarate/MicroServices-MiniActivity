from contextlib import asynccontextmanager
from datetime import datetime, timezone
import hashlib
import os
import secrets
import sqlite3
from typing import Optional

from fastapi import FastAPI, HTTPException, status
from pydantic import BaseModel, Field

DB_PATH = os.getenv("DB_PATH", "/app/data/users.db")


def get_connection() -> sqlite3.Connection:
    """Create a SQLite connection with row dictionary access."""
    os.makedirs(os.path.dirname(DB_PATH) or ".", exist_ok=True)
    conn = sqlite3.connect(DB_PATH, timeout=5.0)
    conn.row_factory = sqlite3.Row
    return conn


def init_db() -> None:
    """Initialize the users SQLite database schema."""
    with get_connection() as conn:
        conn.execute(
            """
            CREATE TABLE IF NOT EXISTS users (
                id INTEGER PRIMARY KEY AUTOINCREMENT,
                name TEXT NOT NULL,
                email TEXT NOT NULL UNIQUE,
                password_hash TEXT NOT NULL,
                created_at TEXT NOT NULL,
                updated_at TEXT NOT NULL
            )
            """
        )
        conn.commit()


def hash_password(password: str) -> str:
    """Hash a plaintext password using PBKDF2-HMAC-SHA256 with a random salt."""
    salt = secrets.token_hex(16)
    digest = hashlib.pbkdf2_hmac("sha256", password.encode("utf-8"), salt.encode("utf-8"), 100_000)
    return f"{salt}${digest.hex()}"


def row_to_user(row: sqlite3.Row) -> dict:
    """Convert a SQLite row to a safe JSON-serializable user dictionary."""
    return {
        "id": row["id"],
        "name": row["name"],
        "email": row["email"],
        "created_at": row["created_at"],
        "updated_at": row["updated_at"],
    }


@asynccontextmanager
async def lifespan(_: FastAPI):
    init_db()
    yield


app = FastAPI(title="Users API", version="1.0.0", lifespan=lifespan)


class UserCreate(BaseModel):
    name: str = Field(..., min_length=1, max_length=120)
    email: str = Field(..., min_length=3, max_length=180)
    password: str = Field(..., min_length=4, max_length=128)


class UserUpdate(BaseModel):
    name: Optional[str] = Field(default=None, min_length=1, max_length=120)
    email: Optional[str] = Field(default=None, min_length=3, max_length=180)
    password: Optional[str] = Field(default=None, min_length=4, max_length=128)


@app.get("/health")
def health():
    with get_connection() as conn:
        conn.execute("SELECT 1")
    return {"status": "ok", "service": "users-api", "database": os.path.basename(DB_PATH)}


@app.post("/users", status_code=status.HTTP_201_CREATED)
def create_user(payload: UserCreate):
    now = datetime.now(timezone.utc).isoformat()
    password_hash = hash_password(payload.password)
    try:
        with get_connection() as conn:
            cursor = conn.execute(
                """
                INSERT INTO users (name, email, password_hash, created_at, updated_at)
                VALUES (?, ?, ?, ?, ?)
                """,
                (payload.name.strip(), payload.email.strip().lower(), password_hash, now, now),
            )
            conn.commit()
            user_id = cursor.lastrowid
            row = conn.execute("SELECT * FROM users WHERE id = ?", (user_id,)).fetchone()
    except sqlite3.IntegrityError:
        raise HTTPException(
            status_code=status.HTTP_409_CONFLICT,
            detail="El correo electrónico ya está registrado.",
        )

    return {"message": "Usuario registrado exitosamente", "user": row_to_user(row)}


@app.get("/users")
def list_users():
    with get_connection() as conn:
        rows = conn.execute("SELECT * FROM users ORDER BY id ASC").fetchall()
    return {"users": [row_to_user(row) for row in rows], "total": len(rows)}


@app.get("/users/{user_id}")
def get_user(user_id: int):
    with get_connection() as conn:
        row = conn.execute("SELECT * FROM users WHERE id = ?", (user_id,)).fetchone()
    if row is None:
        raise HTTPException(status_code=status.HTTP_404_NOT_FOUND, detail="Usuario no encontrado")
    return {"user": row_to_user(row)}


@app.put("/users/{user_id}")
def update_user(user_id: int, payload: UserUpdate):
    with get_connection() as conn:
        existing = conn.execute("SELECT * FROM users WHERE id = ?", (user_id,)).fetchone()
        if existing is None:
            raise HTTPException(status_code=status.HTTP_404_NOT_FOUND, detail="Usuario no encontrado")

        new_name = payload.name.strip() if payload.name is not None else existing["name"]
        new_email = payload.email.strip().lower() if payload.email is not None else existing["email"]
        new_password_hash = (
            hash_password(payload.password) if payload.password is not None else existing["password_hash"]
        )
        now = datetime.now(timezone.utc).isoformat()

        try:
            conn.execute(
                """
                UPDATE users
                SET name = ?, email = ?, password_hash = ?, updated_at = ?
                WHERE id = ?
                """,
                (new_name, new_email, new_password_hash, now, user_id),
            )
            conn.commit()
            updated = conn.execute("SELECT * FROM users WHERE id = ?", (user_id,)).fetchone()
        except sqlite3.IntegrityError:
            raise HTTPException(
                status_code=status.HTTP_409_CONFLICT,
                detail="El correo electrónico ya está en uso por otro usuario.",
            )

    return {"message": "Usuario actualizado exitosamente", "user": row_to_user(updated)}


@app.delete("/users/{user_id}")
def delete_user(user_id: int):
    with get_connection() as conn:
        cursor = conn.execute("DELETE FROM users WHERE id = ?", (user_id,))
        conn.commit()
        if cursor.rowcount == 0:
            raise HTTPException(status_code=status.HTTP_404_NOT_FOUND, detail="Usuario no encontrado")
    return {"message": "Usuario eliminado exitosamente", "deleted_id": user_id}
