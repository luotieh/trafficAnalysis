package domain

import "time"

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

type Message struct {
	ID             int64     `json:"id"`
	MessageID      string    `json:"message_id"`
	EventID        string    `json:"event_id"`
	UserID         string    `json:"user_id,omitempty"`
	UserNickname   string    `json:"user_nickname,omitempty"`
	MessageFrom    string    `json:"message_from"`
	MessageType    string    `json:"message_type"`
	MessageContent string    `json:"message_content"`
	RoundID        int       `json:"round_id"`
	CreatedAt      time.Time `json:"created_at"`
}

type Task struct {
	ID              int64     `json:"id"`
	TaskID          string    `json:"task_id"`
	EventID         string    `json:"event_id"`
	TaskName        string    `json:"task_name"`
	TaskDescription string    `json:"task_description,omitempty"`
	TaskStatus      string    `json:"task_status"`
	TaskPriority    string    `json:"task_priority,omitempty"`
	AssignedTo      string    `json:"assigned_to,omitempty"`
	RoundID         int       `json:"round_id"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

type Execution struct {
	ID              int64     `json:"id"`
	ExecutionID     string    `json:"execution_id"`
	EventID         string    `json:"event_id"`
	CommandID       string    `json:"command_id,omitempty"`
	ExecutionStatus string    `json:"execution_status"`
	ExecutionResult string    `json:"execution_result,omitempty"`
	CommandName     string    `json:"command_name,omitempty"`
	CommandType     string    `json:"command_type,omitempty"`
	CommandEntity   string    `json:"command_entity,omitempty"`
	CommandParams   string    `json:"command_params,omitempty"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
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
