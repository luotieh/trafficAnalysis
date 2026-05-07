package store

import "traffic-go/internal/domain"

type Store interface {
	CreateUser(u domain.User) (domain.User, error)
	GetUserByUsername(username string) (domain.User, bool)
	GetUser(userID string) (domain.User, bool)
	ListUsers() []domain.User
	UpdateUser(userID string, patch map[string]any) (domain.User, bool)
	DeleteUser(userID string) bool

	CreateEvent(e domain.Event) (domain.Event, error)
	GetEvent(eventID string) (domain.Event, bool)
	ListEvents() []domain.Event
	UpdateEvent(eventID string, patch map[string]any) (domain.Event, bool)

	AddMessage(m domain.Message) (domain.Message, error)
	ListMessages(eventID string) []domain.Message

	AddTask(t domain.Task) (domain.Task, error)
	UpdateTask(taskID string, patch map[string]any) (domain.Task, bool)
	ListTasks(eventID string) []domain.Task
	AddAction(a domain.Action) (domain.Action, error)
	UpdateAction(actionID string, patch map[string]any) (domain.Action, bool)
	ListActions(eventID string) []domain.Action
	AddCommand(c domain.Command) (domain.Command, error)
	UpdateCommand(commandID string, patch map[string]any) (domain.Command, bool)
	ListCommands(eventID string) []domain.Command
	AddExecution(e domain.Execution) (domain.Execution, error)
	ListExecutions(eventID string) []domain.Execution
	AddSummary(sm domain.Summary) (domain.Summary, error)
	ListSummaries(eventID string) []domain.Summary

	ReserveFingerprint(fp string) bool
	BindEventMap(fp, lyID, deepSOCID string)
	GetEventMap(fp string) (domain.EventMap, bool)

	GetCursor(name string) domain.SyncCursor
	SaveCursor(c domain.SyncCursor)

	AlreadyPushed(id string) bool
	SavePushedEvent(pe domain.PushedEvent)

	AddAuditLog(a domain.AuditLog) domain.AuditLog
}
