package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
)

// TodoWrite manages a task list that the LLM can update.
type TodoWrite struct {
	mu    sync.Mutex
	todos []TodoItem
}

type TodoItem struct {
	ID          string `json:"id"`
	Subject     string `json:"subject"`
	Description string `json:"description,omitempty"`
	Status      string `json:"status"` // pending, in_progress, completed
	ActiveForm  string `json:"activeForm,omitempty"`
}

func (t *TodoWrite) Name() string { return "todo_write" }

func (t *TodoWrite) Description() string {
	return "Manage a task list. Create, update, or list tasks to track progress."
}

func (t *TodoWrite) Parameters() json.RawMessage {
	return schema(map[string]any{
		"action": map[string]any{
			"type":        "string",
			"enum":        []string{"create", "update", "list"},
			"description": "Action to perform: create a new task, update an existing task, or list all tasks",
		},
		"id": map[string]any{
			"type":        "string",
			"description": "Task ID (for update action)",
		},
		"subject": map[string]any{
			"type":        "string",
			"description": "Short task title (for create action)",
		},
		"description": map[string]any{
			"type":        "string",
			"description": "Detailed description of the task",
		},
		"status": map[string]any{
			"type":        "string",
			"enum":        []string{"pending", "in_progress", "completed"},
			"description": "New status (for update action)",
		},
		"activeForm": map[string]any{
			"type":        "string",
			"description": "Present-tense form for spinner (e.g. 'Running tests')",
		},
	}, "action")
}

type todoInput struct {
	Action      string `json:"action"`
	ID          string `json:"id"`
	Subject     string `json:"subject"`
	Description string `json:"description"`
	Status      string `json:"status"`
	ActiveForm  string `json:"activeForm"`
}

var todoCounter int

func (t *TodoWrite) Execute(ctx context.Context, input json.RawMessage) (json.RawMessage, error) {
	var in todoInput
	if err := json.Unmarshal(input, &in); err != nil {
		return nil, err
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	switch in.Action {
	case "create":
		todoCounter++
		item := TodoItem{
			ID:          fmt.Sprintf("task_%d", todoCounter),
			Subject:     in.Subject,
			Description: in.Description,
			Status:      "pending",
			ActiveForm:  in.ActiveForm,
		}
		if item.Subject == "" {
			return nil, fmt.Errorf("subject is required for create")
		}
		t.todos = append(t.todos, item)
		return json.Marshal(fmt.Sprintf("Created task %s: %s", item.ID, item.Subject))

	case "update":
		for i, item := range t.todos {
			if item.ID == in.ID {
				if in.Status != "" {
					t.todos[i].Status = in.Status
				}
				if in.Subject != "" {
					t.todos[i].Subject = in.Subject
				}
				if in.Description != "" {
					t.todos[i].Description = in.Description
				}
				if in.ActiveForm != "" {
					t.todos[i].ActiveForm = in.ActiveForm
				}
				return json.Marshal(fmt.Sprintf("Updated %s: status=%s", in.ID, t.todos[i].Status))
			}
		}
		return nil, fmt.Errorf("task %s not found", in.ID)

	case "list":
		if len(t.todos) == 0 {
			return json.Marshal("No tasks.")
		}
		var b strings.Builder
		for _, item := range t.todos {
			icon := "○"
			switch item.Status {
			case "in_progress":
				icon = "◐"
			case "completed":
				icon = "●"
			}
			b.WriteString(fmt.Sprintf("%s %s (%s)\n", icon, item.Subject, item.Status))
		}
		return json.Marshal(b.String())

	default:
		return nil, fmt.Errorf("unknown action: %s", in.Action)
	}
}
