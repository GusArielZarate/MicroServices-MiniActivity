package com.microservices.auth;

import jakarta.annotation.PostConstruct;
import org.springframework.beans.factory.annotation.Value;
import org.springframework.stereotype.Service;

import java.io.File;
import java.nio.charset.StandardCharsets;
import java.security.MessageDigest;
import java.security.NoSuchAlgorithmException;
import java.security.SecureRandom;
import java.sql.Connection;
import java.sql.DriverManager;
import java.sql.PreparedStatement;
import java.sql.ResultSet;
import java.sql.SQLException;
import java.sql.Statement;
import java.time.Instant;
import java.util.ArrayList;
import java.util.HexFormat;
import java.util.LinkedHashMap;
import java.util.List;
import java.util.Map;
import java.util.UUID;

@Service
public class DatabaseService {

    private final String dbPath;
    private final SecureRandom secureRandom = new SecureRandom();

    public DatabaseService(@Value("${app.db.path:/app/data/auth.db}") String dbPath) {
        this.dbPath = dbPath;
    }

    @PostConstruct
    public void init() throws SQLException {
        File dbFile = new File(dbPath);
        File parent = dbFile.getParentFile();
        if (parent != null && !parent.exists()) {
            parent.mkdirs();
        }

        try (Connection conn = getConnection(); Statement stmt = conn.createStatement()) {
            stmt.execute("""
                CREATE TABLE IF NOT EXISTS auth_credentials (
                    id INTEGER PRIMARY KEY AUTOINCREMENT,
                    username TEXT NOT NULL UNIQUE,
                    password_hash TEXT NOT NULL,
                    created_at TEXT NOT NULL
                )
            """);

            stmt.execute("""
                CREATE TABLE IF NOT EXISTS auth_sessions (
                    id INTEGER PRIMARY KEY AUTOINCREMENT,
                    username TEXT NOT NULL,
                    token TEXT,
                    valid INTEGER NOT NULL,
                    created_at TEXT NOT NULL
                )
            """);
        }

        seedDefaultCredentials();
    }

    public Connection getConnection() throws SQLException {
        Connection conn = DriverManager.getConnection("jdbc:sqlite:" + dbPath);
        try (Statement stmt = conn.createStatement()) {
            stmt.execute("PRAGMA busy_timeout = 5000");
        }
        return conn;
    }

    public String getDatabaseName() {
        return new File(dbPath).getName();
    }

    public boolean checkHealth() {
        try (Connection conn = getConnection();
             Statement stmt = conn.createStatement();
             ResultSet rs = stmt.executeQuery("SELECT 1")) {
            return rs.next();
        } catch (SQLException e) {
            return false;
        }
    }

    public boolean registerCredential(String username, String password) throws SQLException {
        String normalized = username.trim().toLowerCase();
        String now = Instant.now().toString();
        String passwordHash = hashPassword(password);

        try (Connection conn = getConnection();
             PreparedStatement check = conn.prepareStatement("SELECT id FROM auth_credentials WHERE username = ?")) {
            check.setString(1, normalized);
            try (ResultSet rs = check.executeQuery()) {
                if (rs.next()) {
                    return false;
                }
            }

            try (PreparedStatement insert = conn.prepareStatement(
                    "INSERT INTO auth_credentials (username, password_hash, created_at) VALUES (?, ?, ?)")) {
                insert.setString(1, normalized);
                insert.setString(2, passwordHash);
                insert.setString(3, now);
                insert.executeUpdate();
                return true;
            }
        }
    }

    public Map<String, Object> authenticate(String username, String password) throws SQLException {
        String normalized = username.trim().toLowerCase();
        String now = Instant.now().toString();
        boolean isValid = false;
        String token = null;

        try (Connection conn = getConnection()) {
            try (PreparedStatement stmt = conn.prepareStatement(
                    "SELECT password_hash FROM auth_credentials WHERE username = ?")) {
                stmt.setString(1, normalized);
                try (ResultSet rs = stmt.executeQuery()) {
                    if (rs.next()) {
                        String storedHash = rs.getString("password_hash");
                        isValid = verifyPassword(password, storedHash);
                    }
                }
            }

            if (isValid) {
                token = UUID.randomUUID().toString();
            }

            try (PreparedStatement logStmt = conn.prepareStatement(
                    "INSERT INTO auth_sessions (username, token, valid, created_at) VALUES (?, ?, ?, ?)")) {
                logStmt.setString(1, normalized);
                logStmt.setString(2, token);
                logStmt.setInt(3, isValid ? 1 : 0);
                logStmt.setString(4, now);
                logStmt.executeUpdate();
            }
        }

        Map<String, Object> result = new LinkedHashMap<>();
        result.put("valid", isValid);
        result.put("message", isValid ? "Credenciales válidas" : "Credenciales inválidas");
        result.put("username", normalized);
        if (token != null) {
            result.put("token", token);
        }
        result.put("timestamp", now);
        return result;
    }

    public List<Map<String, Object>> listSessions() throws SQLException {
        List<Map<String, Object>> sessions = new ArrayList<>();
        try (Connection conn = getConnection();
             PreparedStatement stmt = conn.prepareStatement(
                     "SELECT id, username, token, valid, created_at FROM auth_sessions ORDER BY id DESC");
             ResultSet rs = stmt.executeQuery()) {
            while (rs.next()) {
                Map<String, Object> row = new LinkedHashMap<>();
                row.put("id", rs.getInt("id"));
                row.put("username", rs.getString("username"));
                row.put("token", rs.getString("token"));
                row.put("valid", rs.getInt("valid") == 1);
                row.put("created_at", rs.getString("created_at"));
                sessions.add(row);
            }
        }
        return sessions;
    }

    private void seedDefaultCredentials() throws SQLException {
        boolean isEmpty = false;
        try (Connection conn = getConnection();
             Statement stmt = conn.createStatement();
             ResultSet rs = stmt.executeQuery("SELECT COUNT(*) FROM auth_credentials")) {
            if (rs.next() && rs.getInt(1) == 0) {
                isEmpty = true;
            }
        }

        if (isEmpty) {
            registerCredential("admin", "admin123");
            registerCredential("usuario", "123456");
        }
    }

    private String hashPassword(String password) {
        byte[] saltBytes = new byte[16];
        secureRandom.nextBytes(saltBytes);
        String salt = HexFormat.of().formatHex(saltBytes);
        return salt + "$" + sha256(salt + ":" + password);
    }

    private boolean verifyPassword(String password, String storedHash) {
        String[] parts = storedHash.split("\\$", 2);
        if (parts.length != 2) {
            return false;
        }
        String computed = sha256(parts[0] + ":" + password);
        return MessageDigest.isEqual(
                computed.getBytes(StandardCharsets.UTF_8),
                parts[1].getBytes(StandardCharsets.UTF_8));
    }

    private String sha256(String input) {
        try {
            MessageDigest digest = MessageDigest.getInstance("SHA-256");
            byte[] hash = digest.digest(input.getBytes(StandardCharsets.UTF_8));
            return HexFormat.of().formatHex(hash);
        } catch (NoSuchAlgorithmException e) {
            throw new IllegalStateException("SHA-256 algorithm unavailable", e);
        }
    }
}
