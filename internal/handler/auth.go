package handler

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/taskflow-backend/internal/handler/response"
	"github.com/taskflow-backend/internal/model"
	"golang.org/x/crypto/bcrypt"
)

type AuthHandler struct {
	db        *pgxpool.Pool
	jwtSecret string
}

func NewAuthHandler(db *pgxpool.Pool, jwtSecret string) *AuthHandler {
	return &AuthHandler{db: db, jwtSecret: jwtSecret}
}

func (h *AuthHandler) Register(w http.ResponseWriter, r *http.Request) {
	var req model.RegisterRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.Error(w, http.StatusBadRequest, "invalid request body")
		return
	}

	// Validation
	fields := map[string]string{}
	req.Name = strings.TrimSpace(req.Name)
	req.Email = strings.TrimSpace(strings.ToLower(req.Email))

	if req.Name == "" {
		fields["name"] = "is required"
	}
	if req.Email == "" {
		fields["email"] = "is required"
	} else if !strings.Contains(req.Email, "@") {
		fields["email"] = "is invalid"
	}
	if req.Password == "" {
		fields["password"] = "is required"
	} else if len(req.Password) < 8 {
		fields["password"] = "must be at least 8 characters"
	}
	if len(fields) > 0 {
		response.ValidationError(w, fields)
		return
	}

	// Check if email already exists
	var exists bool
	err := h.db.QueryRow(r.Context(), "SELECT EXISTS(SELECT 1 FROM users WHERE email=$1)", req.Email).Scan(&exists)
	if err != nil {
		slog.Error("check email exists", "error", err)
		response.InternalError(w)
		return
	}
	if exists {
		response.ValidationError(w, map[string]string{"email": "already registered"})
		return
	}

	// Hash password
	hashed, err := bcrypt.GenerateFromPassword([]byte(req.Password), 12)
	if err != nil {
		slog.Error("hash password", "error", err)
		response.InternalError(w)
		return
	}

	// Insert user
	var user model.User
	err = h.db.QueryRow(r.Context(),
		`INSERT INTO users (name, email, password) VALUES ($1, $2, $3) RETURNING id, name, email, created_at`,
		req.Name, req.Email, string(hashed),
	).Scan(&user.ID, &user.Name, &user.Email, &user.CreatedAt)
	if err != nil {
		slog.Error("insert user", "error", err)
		response.InternalError(w)
		return
	}

	token, err := h.generateToken(user.ID, user.Email)
	if err != nil {
		slog.Error("generate token", "error", err)
		response.InternalError(w)
		return
	}

	slog.Info("user registered", "user_id", user.ID, "email", user.Email)
	response.JSON(w, http.StatusCreated, model.AuthResponse{Token: token, User: &user})
}

func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	var req model.LoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.Error(w, http.StatusBadRequest, "invalid request body")
		return
	}

	fields := map[string]string{}
	req.Email = strings.TrimSpace(strings.ToLower(req.Email))
	if req.Email == "" {
		fields["email"] = "is required"
	}
	if req.Password == "" {
		fields["password"] = "is required"
	}
	if len(fields) > 0 {
		response.ValidationError(w, fields)
		return
	}

	var user model.User
	err := h.db.QueryRow(r.Context(),
		`SELECT id, name, email, password, created_at FROM users WHERE email=$1`,
		req.Email,
	).Scan(&user.ID, &user.Name, &user.Email, &user.Password, &user.CreatedAt)
	if err == pgx.ErrNoRows {
		response.Error(w, http.StatusUnauthorized, "invalid credentials")
		return
	}
	if err != nil {
		slog.Error("query user", "error", err)
		response.InternalError(w)
		return
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(req.Password)); err != nil {
		response.Error(w, http.StatusUnauthorized, "invalid credentials")
		return
	}

	token, err := h.generateToken(user.ID, user.Email)
	if err != nil {
		slog.Error("generate token", "error", err)
		response.InternalError(w)
		return
	}

	slog.Info("user logged in", "user_id", user.ID)
	user.Password = ""
	response.JSON(w, http.StatusOK, model.AuthResponse{Token: token, User: &user})
}

func (h *AuthHandler) generateToken(userID, email string) (string, error) {
	claims := jwt.MapClaims{
		"user_id": userID,
		"email":   email,
		"exp":     time.Now().Add(24 * time.Hour).Unix(),
		"iat":     time.Now().Unix(),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(h.jwtSecret))
}
