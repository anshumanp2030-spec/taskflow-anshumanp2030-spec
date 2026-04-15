package handler

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/taskflow-backend/internal/handler/response"
	"github.com/taskflow-backend/internal/middleware"
	"github.com/taskflow-backend/internal/model"
)

type ProjectHandler struct {
	db *pgxpool.Pool
}

func NewProjectHandler(db *pgxpool.Pool) *ProjectHandler {
	return &ProjectHandler{db: db}
}

// GET /projects
func (h *ProjectHandler) List(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())

	page, limit := getPagination(r)
	offset := (page - 1) * limit

	rows, err := h.db.Query(r.Context(), `
		SELECT DISTINCT p.id, p.name, p.description, p.owner_id, p.created_at
		FROM projects p
		LEFT JOIN tasks t ON t.project_id = p.id
		WHERE p.owner_id = $1 OR t.assignee_id = $1
		ORDER BY p.created_at DESC
		LIMIT $2 OFFSET $3
	`, userID, limit, offset)
	if err != nil {
		slog.Error("list projects", "error", err)
		response.InternalError(w)
		return
	}
	defer rows.Close()

	projects := []model.Project{}
	for rows.Next() {
		var p model.Project
		if err := rows.Scan(&p.ID, &p.Name, &p.Description, &p.OwnerID, &p.CreatedAt); err != nil {
			slog.Error("scan project", "error", err)
			response.InternalError(w)
			return
		}
		projects = append(projects, p)
	}

	var totalCount int
	h.db.QueryRow(r.Context(), `
		SELECT COUNT(DISTINCT p.id)
		FROM projects p
		LEFT JOIN tasks t ON t.project_id = p.id
		WHERE p.owner_id = $1 OR t.assignee_id = $1
	`, userID).Scan(&totalCount)

	totalPages := (totalCount + limit - 1) / limit
	if totalPages == 0 {
		totalPages = 1
	}

	response.JSON(w, http.StatusOK, model.PaginatedResponse{
		Data:       projects,
		Page:       page,
		Limit:      limit,
		TotalCount: totalCount,
		TotalPages: totalPages,
	})
}

// POST /projects
func (h *ProjectHandler) Create(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())

	var req model.CreateProjectRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.Error(w, http.StatusBadRequest, "invalid request body")
		return
	}

	req.Name = strings.TrimSpace(req.Name)
	fields := map[string]string{}
	if req.Name == "" {
		fields["name"] = "is required"
	}
	if len(fields) > 0 {
		response.ValidationError(w, fields)
		return
	}

	var p model.Project
	err := h.db.QueryRow(r.Context(),
		`INSERT INTO projects (name, description, owner_id) VALUES ($1, $2, $3)
		 RETURNING id, name, description, owner_id, created_at`,
		req.Name, req.Description, userID,
	).Scan(&p.ID, &p.Name, &p.Description, &p.OwnerID, &p.CreatedAt)
	if err != nil {
		slog.Error("create project", "error", err)
		response.InternalError(w)
		return
	}

	slog.Info("project created", "project_id", p.ID, "owner_id", userID)
	response.JSON(w, http.StatusCreated, p)
}

// GET /projects/:id
func (h *ProjectHandler) Get(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	projectID := chi.URLParam(r, "id")

	var p model.Project
	err := h.db.QueryRow(r.Context(),
		`SELECT id, name, description, owner_id, created_at FROM projects WHERE id=$1`,
		projectID,
	).Scan(&p.ID, &p.Name, &p.Description, &p.OwnerID, &p.CreatedAt)
	if err == pgx.ErrNoRows {
		response.NotFound(w)
		return
	}
	if err != nil {
		slog.Error("get project", "error", err)
		response.InternalError(w)
		return
	}

	// Check access: owner or has tasks assigned
	if p.OwnerID != userID {
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

	// Fetch tasks
	rows, err := h.db.Query(r.Context(),
		`SELECT id, title, description, status, priority, project_id, assignee_id, to_char(due_date, 'YYYY-MM-DD'), created_at, updated_at
		 FROM tasks WHERE project_id=$1 ORDER BY created_at DESC`,
		projectID,
	)
	if err != nil {
		slog.Error("get project tasks", "error", err)
		response.InternalError(w)
		return
	}
	defer rows.Close()

	tasks := []model.Task{}
	for rows.Next() {
		var t model.Task
		if err := rows.Scan(&t.ID, &t.Title, &t.Description, &t.Status, &t.Priority, &t.ProjectID, &t.AssigneeID, &t.DueDate, &t.CreatedAt, &t.UpdatedAt); err != nil {
			slog.Error("scan task", "error", err)
			response.InternalError(w)
			return
		}
		tasks = append(tasks, t)
	}

	result := model.ProjectWithTasks{Project: p, Tasks: tasks}
	response.JSON(w, http.StatusOK, result)
}

// PATCH /projects/:id
func (h *ProjectHandler) Update(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	projectID := chi.URLParam(r, "id")

	var ownerID string
	err := h.db.QueryRow(r.Context(), `SELECT owner_id FROM projects WHERE id=$1`, projectID).Scan(&ownerID)
	if err == pgx.ErrNoRows {
		response.NotFound(w)
		return
	}
	if err != nil {
		slog.Error("check project owner", "error", err)
		response.InternalError(w)
		return
	}
	if ownerID != userID {
		response.Forbidden(w)
		return
	}

	var req model.UpdateProjectRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.Error(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Name != nil {
		*req.Name = strings.TrimSpace(*req.Name)
		if *req.Name == "" {
			response.ValidationError(w, map[string]string{"name": "cannot be empty"})
			return
		}
	}

	var p model.Project
	err = h.db.QueryRow(r.Context(),
		`UPDATE projects SET
			name = COALESCE($1, name),
			description = CASE WHEN $2::boolean THEN $3 ELSE description END
		 WHERE id=$4
		 RETURNING id, name, description, owner_id, created_at`,
		req.Name, req.Description != nil, req.Description, projectID,
	).Scan(&p.ID, &p.Name, &p.Description, &p.OwnerID, &p.CreatedAt)
	if err != nil {
		slog.Error("update project", "error", err)
		response.InternalError(w)
		return
	}

	response.JSON(w, http.StatusOK, p)
}

// DELETE /projects/:id
func (h *ProjectHandler) Delete(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	projectID := chi.URLParam(r, "id")

	var ownerID string
	err := h.db.QueryRow(r.Context(), `SELECT owner_id FROM projects WHERE id=$1`, projectID).Scan(&ownerID)
	if err == pgx.ErrNoRows {
		response.NotFound(w)
		return
	}
	if err != nil {
		slog.Error("check project owner for delete", "error", err)
		response.InternalError(w)
		return
	}
	if ownerID != userID {
		response.Forbidden(w)
		return
	}

	_, err = h.db.Exec(r.Context(), `DELETE FROM projects WHERE id=$1`, projectID)
	if err != nil {
		slog.Error("delete project", "error", err)
		response.InternalError(w)
		return
	}

	slog.Info("project deleted", "project_id", projectID, "by", userID)
	response.NoContent(w)
}

// GET /projects/:id/stats
func (h *ProjectHandler) Stats(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	projectID := chi.URLParam(r, "id")

	var ownerID string
	err := h.db.QueryRow(r.Context(), `SELECT owner_id FROM projects WHERE id=$1`, projectID).Scan(&ownerID)
	if err == pgx.ErrNoRows {
		response.NotFound(w)
		return
	}
	if err != nil {
		slog.Error("check project for stats", "error", err)
		response.InternalError(w)
		return
	}
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

	// By status
	statusRows, err := h.db.Query(r.Context(),
		`SELECT status, COUNT(*) FROM tasks WHERE project_id=$1 GROUP BY status`, projectID)
	if err != nil {
		slog.Error("stats by status", "error", err)
		response.InternalError(w)
		return
	}
	defer statusRows.Close()

	byStatus := map[string]int{"todo": 0, "in_progress": 0, "done": 0}
	for statusRows.Next() {
		var s string
		var cnt int
		statusRows.Scan(&s, &cnt)
		byStatus[s] = cnt
	}

	// By assignee
	assigneeRows, err := h.db.Query(r.Context(),
		`SELECT t.assignee_id, u.name, COUNT(t.id)
		 FROM tasks t
		 LEFT JOIN users u ON u.id = t.assignee_id
		 WHERE t.project_id=$1
		 GROUP BY t.assignee_id, u.name`, projectID)
	if err != nil {
		slog.Error("stats by assignee", "error", err)
		response.InternalError(w)
		return
	}
	defer assigneeRows.Close()

	byAssignee := []model.AssigneeStat{}
	for assigneeRows.Next() {
		var s model.AssigneeStat
		assigneeRows.Scan(&s.AssigneeID, &s.AssigneeName, &s.Count)
		byAssignee = append(byAssignee, s)
	}

	response.JSON(w, http.StatusOK, model.ProjectStats{
		ProjectID:  projectID,
		ByStatus:   byStatus,
		ByAssignee: byAssignee,
	})
}

func getPagination(r *http.Request) (page, limit int) {
	page, _ = strconv.Atoi(r.URL.Query().Get("page"))
	limit, _ = strconv.Atoi(r.URL.Query().Get("limit"))
	if page < 1 {
		page = 1
	}
	if limit < 1 || limit > 100 {
		limit = 20
	}
	return
}
