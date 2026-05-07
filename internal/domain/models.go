package domain

import (
	"strings"
	"time"
)

type APIResponse struct {
	Status  string      `json:"status,omitempty"`
	Message string      `json:"message,omitempty"`
	Data    interface{} `json:"data,omitempty"`
}

type User struct {
	ID          int64      `json:"id"`
	UserID      string     `json:"user_id"`
	Username    string     `json:"username"`
	Nickname    string     `json:"nickname,omitempty"`
	Email       string     `json:"email,omitempty"`
	Phone       string     `json:"phone,omitempty"`
	Password    string     `json:"-"`
	Role        string     `json:"role"`
	IsActive    bool       `json:"is_active"`
	LastLoginAt *time.Time `json:"last_login_at,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

type Event struct {
	ID           int64     `json:"id"`
	EventID      string    `json:"event_id"`
	EventName    string    `json:"event_name,omitempty"`
	Title        string    `json:"title,omitempty"`
	Message      string    `json:"message"`
	Context      string    `json:"context,omitempty"`
	Source       string    `json:"source,omitempty"`
	Severity     string    `json:"severity"`
	Category     string    `json:"category,omitempty"`
	EventStatus  string    `json:"event_status"`
	CurrentRound int       `json:"current_round"`
	Observables  []IOC     `json:"observables,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

type IOC struct {
	Type  string `json:"type"`
	Value string `json:"value"`
	Role  string `json:"role,omitempty"`
}

const (
	RoleSystem    = "system"
	RoleUser      = "user"
	RoleAssistant = "assistant"
	RoleCaptain   = "_captain"
	RoleManager   = "_manager"
	RoleOperator  = "_operator"
	RoleExecutor  = "_executor"
	RoleExpert    = "_expert"
)

var AgentRoles = []string{RoleCaptain, RoleManager, RoleOperator, RoleExecutor, RoleExpert}

func NormalizeMessageFrom(v string) string {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "", "system":
		return RoleSystem
	case "user", "engineer", "human":
		return RoleUser
	case "assistant", "ai", "ai_assistant", "engineer_ai":
		return RoleAssistant
	case "captain", "role_soc_captain", "soc_captain", "_captain":
		return RoleCaptain
	case "manager", "role_soc_manager", "soc_manager", "_manager":
		return RoleManager
	case "operator", "role_soc_operator", "soc_operator", "_operator":
		return RoleOperator
	case "executor", "role_soc_executor", "soc_executor", "_executor", "autopilot", "queue-worker":
		return RoleExecutor
	case "expert", "role_soc_expert", "soc_expert", "_expert":
		return RoleExpert
	default:
		return strings.TrimSpace(v)
	}
}

func SenderType(messageFrom string) string {
	switch NormalizeMessageFrom(messageFrom) {
	case RoleUser:
		return "user"
	case RoleAssistant:
		return "ai"
	case RoleSystem:
		return "system"
	default:
		return "agent"
	}
}

func NormalizeMessage(m Message) Message {
	m.MessageFrom = NormalizeMessageFrom(m.MessageFrom)
	if m.SenderType == "" || m.SenderType == "unknown" {
		m.SenderType = SenderType(m.MessageFrom)
	}
	if m.MessageCategory == "" {
		if m.SenderType == "user" || m.SenderType == "ai" {
			m.MessageCategory = "engineer_chat"
		} else {
			m.MessageCategory = "agent"
		}
	}
	return m
}

type Message struct {
	ID              int64     `json:"id"`
	MessageID       string    `json:"message_id"`
	EventID         string    `json:"event_id"`
	UserID          string    `json:"user_id,omitempty"`
	UserNickname    string    `json:"user_nickname,omitempty"`
	MessageFrom     string    `json:"message_from"`
	MessageType     string    `json:"message_type"`
	MessageContent  string    `json:"message_content"`
	RoundID         int       `json:"round_id"`
	MessageCategory string    `json:"message_category,omitempty"`
	SenderType      string    `json:"sender_type,omitempty"`
	ChatSessionID   string    `json:"chat_session_id,omitempty"`
	CreatedAt       time.Time `json:"created_at"`
}

type Task struct {
	ID              int64     `json:"id"`
	TaskID          string    `json:"task_id"`
	EventID         string    `json:"event_id"`
	TaskName        string    `json:"task_name"`
	TaskType        string    `json:"task_type,omitempty"`
	TaskDescription string    `json:"task_description,omitempty"`
	TaskStatus      string    `json:"task_status"`
	TaskPriority    string    `json:"task_priority,omitempty"`
	AssignedTo      string    `json:"assigned_to,omitempty"`
	TaskAssignee    string    `json:"task_assignee,omitempty"`
	RoundID         int       `json:"round_id"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

type Action struct {
	ID             int64     `json:"id"`
	ActionID       string    `json:"action_id"`
	TaskID         string    `json:"task_id"`
	EventID        string    `json:"event_id"`
	RoundID        int       `json:"round_id"`
	ActionName     string    `json:"action_name"`
	ActionType     string    `json:"action_type,omitempty"`
	ActionAssignee string    `json:"action_assignee,omitempty"`
	ActionStatus   string    `json:"action_status"`
	ActionResult   string    `json:"action_result,omitempty"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

type Command struct {
	ID              int64     `json:"id"`
	CommandID       string    `json:"command_id"`
	ActionID        string    `json:"action_id"`
	TaskID          string    `json:"task_id"`
	EventID         string    `json:"event_id"`
	RoundID         int       `json:"round_id"`
	CommandName     string    `json:"command_name"`
	CommandType     string    `json:"command_type,omitempty"`
	CommandAssignee string    `json:"command_assignee,omitempty"`
	CommandEntity   string    `json:"command_entity,omitempty"`
	CommandParams   string    `json:"command_params,omitempty"`
	CommandStatus   string    `json:"command_status"`
	CommandResult   string    `json:"command_result,omitempty"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

type Execution struct {
	ID               int64     `json:"id"`
	ExecutionID      string    `json:"execution_id"`
	EventID          string    `json:"event_id"`
	TaskID           string    `json:"task_id,omitempty"`
	ActionID         string    `json:"action_id,omitempty"`
	RoundID          int       `json:"round_id,omitempty"`
	CommandID        string    `json:"command_id,omitempty"`
	ExecutionStatus  string    `json:"execution_status"`
	ExecutionResult  string    `json:"execution_result,omitempty"`
	ExecutionSummary string    `json:"execution_summary,omitempty"`
	AISummary        string    `json:"ai_summary,omitempty"`
	CommandName      string    `json:"command_name,omitempty"`
	CommandType      string    `json:"command_type,omitempty"`
	CommandEntity    string    `json:"command_entity,omitempty"`
	CommandParams    string    `json:"command_params,omitempty"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
}

type Summary struct {
	ID           int64     `json:"id"`
	EventID      string    `json:"event_id"`
	RoundID      int       `json:"round_id"`
	EventSummary string    `json:"event_summary"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

type LyEvent map[string]any

type EventMap struct {
	Fingerprint    string    `json:"fingerprint"`
	LyEventID      string    `json:"ly_event_id,omitempty"`
	DeepSOCEventID string    `json:"deepsoc_event_id,omitempty"`
	CreatedAt      time.Time `json:"created_at"`
}

type SyncCursor struct {
	Name      string    `json:"name"`
	LastTS    string    `json:"last_ts"`
	UpdatedAt time.Time `json:"updated_at"`
}

type PushedEvent struct {
	LyEventID      string    `json:"ly_event_id"`
	IdempotencyKey string    `json:"idempotency_key"`
	DeepSOCEventID string    `json:"deepsoc_event_id"`
	Status         string    `json:"status"`
	Attempts       int       `json:"attempts"`
	LastError      string    `json:"last_error"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

type AuditLog struct {
	ID        int64     `json:"id"`
	Actor     string    `json:"actor"`
	Action    string    `json:"action"`
	Target    string    `json:"target"`
	Meta      string    `json:"meta,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}
