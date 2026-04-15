package handler

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/taskflow-backend/internal/handler/response"
	"github.com/taskflow-backend/internal/middleware"
	"github.com/taskflow-backend/internal/model"
)

type TaskHandler struct {
	db *pgxpool.Pool
}

func NewTaskHandler(db *pgxpool.Pool) *TaskHandler {
	return &TaskHandler{db: db}
}

// GET /projects/:id/tasks
func (h *TaskHandler) List(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	projectID := chi.URLParam(r, "id")

	// Check project access
	if !h.canAccessProject(r, projectID, userID) {
		response.Forbidden(w)
		return
	}

	statusFilter := r.URL.Query().Get("status")
	assigneeFilter := r.URL.Query().Get("assignee")
	page, limit := getPagination(r)
	offset := (page - 1) * limit

	query := `
		SELECT id, title, description, status, priority, project_id, assignee_id,
		       to_char(due_date, 'YYYY-MM-DD'), created_at, updated_at
		FROM tasks
		WHERE project_id = $1
	`
	args := []interface{}{projectID}
	argIdx := 2

	if statusFilter != "" {
		query += " AND status = $" + itoa(argIdx)
		args = append(args, statusFilter)
		argIdx++
	}
	if assigneeFilter != "" {
		query += " AND assignee_id = $" + itoa(argIdx)
		args = append(args, assigneeFilter)
		argIdx++
	}

	countQuery := strings.Replace(query,
		`id, title, description, status, priority, project_id, assignee_id,
		       to_char(due_date, 'YYYY-MM-DD'), created_at, updated_at`,
		"COUNT(*)", 1)

	var totalCount int
	h.db.QueryRow(r.Context(), countQuery, args...).Scan(&totalCount)

	query += " ORDER BY created_at DESC LIMIT $" + itoa(argIdx) + " OFFSET $" + itoa(argIdx+1)
	args = append(args, limit, offset)

	rows, err := h.db.Query(r.Context(), query, args...)
	if err != nil {
		slog.Error("list tasks", "error", err)
		response.InternalError(w)
		return
	}
	defer rows.Close()

	tasks := []model.Task{}
	for rows.Next() {
		var t model.Task
		if err := rows.Scan(&t.ID, &t.Title, &t.Description, &t.Status, &t.Priority, &t.ProjectID, &t.AssigneeID, &t.DueDate, &t.CreatedAt, &t.UpdatedAt); err != nil {
			slog.Error("scan task in list", "error", err)
			response.InternalError(w)
			return
		}
		tasks = append(tasks, t)
	}

	totalPages := (totalCount + limit - 1) / limit
	if totalPages == 0 {
		totalPages = 1
	}

	response.JSON(w, http.StatusOK, model.PaginatedResponse{
		Data:       tasks,
		Page:       page,
		Limit:      limit,
		TotalCount: totalCount,
		TotalPages: totalPages,
	})
}

// POST /projects/:id/tasks
func (h *TaskHandler) Create(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	projectID := chi.URLParam(r, "id")

	// Project must exist and user must have access
	if !h.canAccessProject(r, projectID, userID) {
		response.Forbidden(w)
		return
	}

	var req model.CreateTaskRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.Error(w, http.StatusBadRequest, "invalid request body")
		return
	}

	req.Title = strings.TrimSpace(req.Title)
	fields := map[string]string{}

	if req.Title == "" {
		fields["title"] = "is required"
	}
	if req.Priority == "" {
		req.Priority = model.PriorityMedium
	} else if !req.Priority.Valid() {
		fields["priority"] = "must be low, medium, or high"
	}
	if len(fields) > 0 {
		response.ValidationError(w, fields)
		return
	}

	// Validate assignee exists if provided
	if req.AssigneeID != nil {
		var exists bool
		h.db.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM users WHERE id=$1)`, *req.AssigneeID).Scan(&exists)
		if !exists {
			response.ValidationError(w, map[string]string{"assignee_id": "user not found"})
			return
		}
	}

	var t model.Task
	err := h.db.QueryRow(r.Context(),
		`INSERT INTO tasks (title, description, status, priority, project_id, assignee_id, due_date)
		 VALUES ($1, $2, $3, $4, $5, $6, $7::date)
		 RETURNING id, title, description, status, priority, project_id, assignee_id,
		           to_char(due_date, 'YYYY-MM-DD'), created_at, updated_at`,
		req.Title, req.Description, model.StatusTodo, req.Priority, projectID, req.AssigneeID, req.DueDate,
	).Scan(&t.ID, &t.Title, &t.Description, &t.Status, &t.Priority, &t.ProjectID, &t.AssigneeID, &t.DueDate, &t.CreatedAt, &t.UpdatedAt)
	if err != nil {
		slog.Error("create task", "error", err)
		response.InternalError(w)
		return
	}

	slog.Info("task created", "task_id", t.ID, "project_id", projectID)
	response.JSON(w, http.StatusCreated, t)
}

// PATCH /tasks/:id
func (h *TaskHandler) Update(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	taskID := chi.URLParam(r, "id")

	var projectID string
	err := h.db.QueryRow(r.Context(),
		`SELECT project_id FROM tasks WHERE id=$1`, taskID,
	).Scan(&projectID)
	if err == pgx.ErrNoRows {
		response.NotFound(w)
		return
	}
	if err != nil {
		slog.Error("get task for update", "error", err)
		response.InternalError(w)
		return
	}

	// Check access: must be project member
	if !h.canAccessProject(r, projectID, userID) {
		response.Forbidden(w)
		return
	}

	var req model.UpdateTaskRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.Error(w, http.StatusBadRequest, "invalid request body")
		return
	}

	fields := map[string]string{}
	if req.Title != nil {
		*req.Title = strings.TrimSpace(*req.Title)
		if *req.Title == "" {
			fields["title"] = "cannot be empty"
		}
	}
	if req.Status != nil && !req.Status.Valid() {
		fields["status"] = "must be todo, in_progress, or done"
	}
	if req.Priority != nil && !req.Priority.Valid() {
		fields["priority"] = "must be low, medium, or high"
	}
	if len(fields) > 0 {
		response.ValidationError(w, fields)
		return
	}

	// Validate assignee if provided
	if req.AssigneeID != nil && *req.AssigneeID != "" {
		var exists bool
		h.db.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM users WHERE id=$1)`, *req.AssigneeID).Scan(&exists)
		if !exists {
			response.ValidationError(w, map[string]string{"assignee_id": "user not found"})
			return
		}
	}

	var t model.Task
	err = h.db.QueryRow(r.Context(),
		`UPDATE tasks SET
			title       = COALESCE($1, title),
			description = CASE WHEN $2::boolean THEN $3 ELSE description END,
			status      = COALESCE($4, status),
			priority    = COALESCE($5, priority),
			assignee_id = CASE WHEN $6::boolean THEN $7::uuid ELSE assignee_id END,
			due_date    = CASE WHEN $8::boolean THEN $9::date ELSE due_date END,
			updated_at  = NOW()
		 WHERE id=$10
		 RETURNING id, title, description, status, priority, project_id, assignee_id,
		           to_char(due_date, 'YYYY-MM-DD'), created_at, updated_at`,
		req.Title,
		req.Description != nil, req.Description,
		req.Status,
		req.Priority,
		req.AssigneeID != nil, req.AssigneeID,
		req.DueDate != nil, req.DueDate,
		taskID,
	).Scan(&t.ID, &t.Title, &t.Description, &t.Status, &t.Priority, &t.ProjectID, &t.AssigneeID, &t.DueDate, &t.CreatedAt, &t.UpdatedAt)
	if err != nil {
		slog.Error("update task", "error", err)
		response.InternalError(w)
		return
	}

	response.JSON(w, http.StatusOK, t)
}

// DELETE /tasks/:id
func (h *TaskHandler) Delete(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	taskID := chi.URLParam(r, "id")

	var projectID string
	err := h.db.QueryRow(r.Context(),
		`SELECT project_id FROM tasks WHERE id=$1`, taskID,
	).Scan(&projectID)
	if err == pgx.ErrNoRows {
		response.NotFound(w)
		return
	}
	if err != nil {
		slog.Error("get task for delete", "error", err)
		response.InternalError(w)
		return
	}

	// Check: project owner or has access
	var ownerID string
	h.db.QueryRow(r.Context(), `SELECT owner_id FROM projects WHERE id=$1`, projectID).Scan(&ownerID)
	if ownerID != userID {
		var hasAccess bool
		h.db.QueryRow(r.Context(),
			`SELECT EXISTS(SELECT 1 FROM tasks WHERE project_id=$1 AND assignee_id=$2)`,
			projectID, userID,
		).Scan(&hasAccess)
		if !hasAccess {
			response.Forbidden(w)
			return
		}
	}

	_, err = h.db.Exec(r.Context(), `DELETE FROM tasks WHERE id=$1`, taskID)
	if err != nil {
		slog.Error("delete task", "error", err)
		response.InternalError(w)
		return
	}

	slog.Info("task deleted", "task_id", taskID, "by", userID)
	response.NoContent(w)
}

func (h *TaskHandler) canAccessProject(r *http.Request, projectID, userID string) bool {
	var ownerID string
	err := h.db.QueryRow(r.Context(), `SELECT owner_id FROM projects WHERE id=$1`, projectID).Scan(&ownerID)
	if err != nil {
		return false
	}
	if ownerID == userID {
		return true
	}
	var hasTask bool
	h.db.QueryRow(r.Context(),
		`SELECT EXISTS(SELECT 1 FROM tasks WHERE project_id=$1 AND assignee_id=$2)`,
		projectID, userID,
	).Scan(&hasTask)
	return hasTask
}

func itoa(n int) string {
	return fmt.Sprintf("%d", n)
}
