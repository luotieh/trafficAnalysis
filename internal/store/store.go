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

	ListTasks(eventID string) []domain.Task
	ListExecutions(eventID string) []domain.Execution
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
