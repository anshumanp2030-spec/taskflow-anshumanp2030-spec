package model

import (
	"time"
)

type User struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Email     string    `json:"email"`
	Password  string    `json:"-"`
	CreatedAt time.Time `json:"created_at"`
}

type Project struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Description *string   `json:"description,omitempty"`
	OwnerID     string    `json:"owner_id"`
	CreatedAt   time.Time `json:"created_at"`
}

type ProjectWithTasks struct {
	Project
	Tasks []Task `json:"tasks"`
}

type ProjectStats struct {
	ProjectID  string         `json:"project_id"`
	ByStatus   map[string]int `json:"by_status"`
	ByAssignee []AssigneeStat `json:"by_assignee"`
}

type AssigneeStat struct {
	AssigneeID   *string `json:"assignee_id"`
	AssigneeName *string `json:"assignee_name"`
	Count        int     `json:"count"`
}

type TaskStatus string
type TaskPriority string

const (
	StatusTodo       TaskStatus = "todo"
	StatusInProgress TaskStatus = "in_progress"
	StatusDone       TaskStatus = "done"

	PriorityLow    TaskPriority = "low"
	PriorityMedium TaskPriority = "medium"
	PriorityHigh   TaskPriority = "high"
)

func (s TaskStatus) Valid() bool {
	switch s {
	case StatusTodo, StatusInProgress, StatusDone:
		return true
	}
	return false
}

func (p TaskPriority) Valid() bool {
	switch p {
	case PriorityLow, PriorityMedium, PriorityHigh:
		return true
	}
	return false
}

type Task struct {
	ID          string       `json:"id"`
	Title       string       `json:"title"`
	Description *string      `json:"description,omitempty"`
	Status      TaskStatus   `json:"status"`
	Priority    TaskPriority `json:"priority"`
	ProjectID   string       `json:"project_id"`
	AssigneeID  *string      `json:"assignee_id,omitempty"`
	DueDate     *string      `json:"due_date,omitempty"`
	CreatedAt   time.Time    `json:"created_at"`
	UpdatedAt   time.Time    `json:"updated_at"`
}

// Request/Response types

type RegisterRequest struct {
	Name     string `json:"name"`
	Email    string `json:"email"`
	Password string `json:"password"`
}

type LoginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type AuthResponse struct {
	Token string `json:"token"`
	User  *User  `json:"user"`
}

type CreateProjectRequest struct {
	Name        string  `json:"name"`
	Description *string `json:"description"`
}

type UpdateProjectRequest struct {
	Name        *string `json:"name"`
	Description *string `json:"description"`
}

type CreateTaskRequest struct {
	Title       string       `json:"title"`
	Description *string      `json:"description"`
	Priority    TaskPriority `json:"priority"`
	AssigneeID  *string      `json:"assignee_id"`
	DueDate     *string      `json:"due_date"`
}

type UpdateTaskRequest struct {
	Title       *string       `json:"title"`
	Description *string       `json:"description"`
	Status      *TaskStatus   `json:"status"`
	Priority    *TaskPriority `json:"priority"`
	AssigneeID  *string       `json:"assignee_id"`
	DueDate     *string       `json:"due_date"`
}

// Pagination
type PaginationParams struct {
	Page  int
	Limit int
}

type PaginatedResponse struct {
	Data       interface{} `json:"data"`
	Page       int         `json:"page"`
	Limit      int         `json:"limit"`
	TotalCount int         `json:"total_count"`
	TotalPages int         `json:"total_pages"`
}
