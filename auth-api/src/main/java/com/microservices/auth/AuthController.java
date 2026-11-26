package com.microservices.auth;

import org.springframework.http.HttpStatus;
import org.springframework.http.ResponseEntity;
import org.springframework.web.bind.annotation.GetMapping;
import org.springframework.web.bind.annotation.PostMapping;
import org.springframework.web.bind.annotation.RequestBody;
import org.springframework.web.bind.annotation.RestController;

import java.sql.SQLException;
import java.util.LinkedHashMap;
import java.util.List;
import java.util.Map;

@RestController
public class AuthController {

    private final DatabaseService databaseService;

    public AuthController(DatabaseService databaseService) {
        this.databaseService = databaseService;
    }

    @GetMapping({"/health", "/status"})
    public ResponseEntity<Map<String, Object>> health() {
        boolean dbOk = databaseService.checkHealth();
        Map<String, Object> body = new LinkedHashMap<>();
        body.put("status", dbOk ? "ok" : "error");
        body.put("service", "auth-api");
        body.put("database", databaseService.getDatabaseName());
        return ResponseEntity.status(dbOk ? HttpStatus.OK : HttpStatus.INTERNAL_SERVER_ERROR).body(body);
    }

    @PostMapping("/login")
    public ResponseEntity<Map<String, Object>> login(@RequestBody Map<String, String> payload) {
        String username = payload.get("username");
        if (username == null || username.isBlank()) {
            username = payload.get("email");
        }
        String password = payload.get("password");

        if (username == null || username.isBlank() || password == null || password.isBlank()) {
            return ResponseEntity.badRequest().body(Map.of(
                    "valid", false,
                    "message", "Se requieren los campos 'username' (o 'email') y 'password'."
            ));
        }

        try {
            Map<String, Object> authResult = databaseService.authenticate(username, password);
            boolean valid = Boolean.TRUE.equals(authResult.get("valid"));
            return ResponseEntity.status(valid ? HttpStatus.OK : HttpStatus.UNAUTHORIZED).body(authResult);
        } catch (SQLException e) {
            return ResponseEntity.status(HttpStatus.INTERNAL_SERVER_ERROR).body(Map.of(
                    "valid", false,
                    "message", "Error al consultar la base de datos de autenticación."
            ));
        }
    }

    @PostMapping("/register")
    public ResponseEntity<Map<String, Object>> register(@RequestBody Map<String, String> payload) {
        String username = payload.get("username");
        if (username == null || username.isBlank()) {
            username = payload.get("email");
        }
        String password = payload.get("password");

        if (username == null || username.isBlank() || password == null || password.length() < 4) {
            return ResponseEntity.badRequest().body(Map.of(
                    "message", "Se requiere 'username' (o 'email') y 'password' de al menos 4 caracteres."
            ));
        }

        try {
            boolean created = databaseService.registerCredential(username, password);
            if (!created) {
                return ResponseEntity.status(HttpStatus.CONFLICT).body(Map.of(
                        "message", "El usuario ya existe en auth.db."
                ));
            }
            return ResponseEntity.status(HttpStatus.CREATED).body(Map.of(
                    "message", "Credenciales registradas exitosamente",
                    "username", username.trim().toLowerCase()
            ));
        } catch (SQLException e) {
            return ResponseEntity.status(HttpStatus.INTERNAL_SERVER_ERROR).body(Map.of(
                    "message", "Error al registrar credenciales."
            ));
        }
    }

    @GetMapping("/sessions")
    public ResponseEntity<Map<String, Object>> sessions() {
        try {
            List<Map<String, Object>> sessions = databaseService.listSessions();
            return ResponseEntity.ok(Map.of(
                    "sessions", sessions,
                    "total", sessions.size()
            ));
        } catch (SQLException e) {
            return ResponseEntity.status(HttpStatus.INTERNAL_SERVER_ERROR).body(Map.of(
                    "message", "Error al consultar registros de sesiones."
            ));
        }
    }
}
