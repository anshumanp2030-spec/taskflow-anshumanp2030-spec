package response

import (
	"encoding/json"
	"net/http"
)

func JSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

func Error(w http.ResponseWriter, status int, msg string) {
	JSON(w, status, map[string]string{"error": msg})
}

func ValidationError(w http.ResponseWriter, fields map[string]string) {
	JSON(w, http.StatusBadRequest, map[string]interface{}{
		"error":  "validation failed",
		"fields": fields,
	})
}

func Unauthorized(w http.ResponseWriter) {
	Error(w, http.StatusUnauthorized, "unauthorized")
}

func Forbidden(w http.ResponseWriter) {
	Error(w, http.StatusForbidden, "forbidden")
}

func NotFound(w http.ResponseWriter) {
	Error(w, http.StatusNotFound, "not found")
}

func InternalError(w http.ResponseWriter) {
	Error(w, http.StatusInternalServerError, "internal server error")
}

func NoContent(w http.ResponseWriter) {
	w.WriteHeader(http.StatusNoContent)
}
