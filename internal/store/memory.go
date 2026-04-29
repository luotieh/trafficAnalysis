package store

import (
	"errors"
	"sort"
	"sync"
	"time"

	"traffic-go/internal/domain"
)

type MemoryStore struct {
	mu sync.RWMutex

	usersByID       map[string]domain.User
	usersByUsername map[string]string
	userSeq         int64

	events   map[string]domain.Event
	eventSeq int64

	messagesByEvent map[string][]domain.Message
	messageSeq      int64

	tasksByEvent     map[string][]domain.Task
	execByEvent      map[string][]domain.Execution
	summariesByEvent map[string][]domain.Summary
	eventMaps        map[string]domain.EventMap
	cursors          map[string]domain.SyncCursor
	pushed           map[string]domain.PushedEvent
	audits           []domain.AuditLog
	auditSeq         int64
}

func NewMemoryStore() *MemoryStore {
	s := &MemoryStore{
		usersByID:        map[string]domain.User{},
		usersByUsername:  map[string]string{},
		events:           map[string]domain.Event{},
		messagesByEvent:  map[string][]domain.Message{},
		tasksByEvent:     map[string][]domain.Task{},
		execByEvent:      map[string][]domain.Execution{},
		summariesByEvent: map[string][]domain.Summary{},
		eventMaps:        map[string]domain.EventMap{},
		cursors:          map[string]domain.SyncCursor{},
		pushed:           map[string]domain.PushedEvent{},
	}
	now := time.Now().UTC()
	admin := domain.User{
		ID:        1,
		UserID:    "admin",
		Username:  "admin",
		Nickname:  "管理员",
		Email:     "admin@example.local",
		Role:      "admin",
		IsActive:  true,
		Password:  "admin",
		CreatedAt: now,
		UpdatedAt: now,
	}
	s.userSeq = 1
	s.usersByID[admin.UserID] = admin
	s.usersByUsername[admin.Username] = admin.UserID
	return s
}

func (s *MemoryStore) CreateUser(u domain.User) (domain.User, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.usersByUsername[u.Username]; ok {
		return domain.User{}, errors.New("username already exists")
	}
	now := time.Now().UTC()
	s.userSeq++
	if u.UserID == "" {
		u.UserID = newID("u")
	}
	u.ID = s.userSeq
	u.IsActive = true
	if u.Role == "" {
		u.Role = "user"
	}
	if u.CreatedAt.IsZero() {
		u.CreatedAt = now
	}
	u.UpdatedAt = now
	s.usersByID[u.UserID] = u
	s.usersByUsername[u.Username] = u.UserID
	return u, nil
}

func (s *MemoryStore) GetUserByUsername(username string) (domain.User, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	id, ok := s.usersByUsername[username]
	if !ok {
		return domain.User{}, false
	}
	u, ok := s.usersByID[id]
	return u, ok
}

func (s *MemoryStore) GetUser(userID string) (domain.User, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	u, ok := s.usersByID[userID]
	return u, ok
}

func (s *MemoryStore) ListUsers() []domain.User {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]domain.User, 0, len(s.usersByID))
	for _, u := range s.usersByID {
		out = append(out, u)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

func (s *MemoryStore) UpdateUser(userID string, patch map[string]any) (domain.User, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	u, ok := s.usersByID[userID]
	if !ok {
		return domain.User{}, false
	}
	if v, ok := stringPatch(patch, "nickname"); ok {
		u.Nickname = v
	}
	if v, ok := stringPatch(patch, "email"); ok {
		u.Email = v
	}
	if v, ok := stringPatch(patch, "phone"); ok {
		u.Phone = v
	}
	if v, ok := stringPatch(patch, "role"); ok {
		u.Role = v
	}
	if v, ok := boolPatch(patch, "is_active"); ok {
		u.IsActive = v
	}
	if v, ok := stringPatch(patch, "password"); ok {
		u.Password = v
	}
	if t, ok := timePatch(patch, "last_login_at"); ok {
		u.LastLoginAt = t
	}
	u.UpdatedAt = time.Now().UTC()
	s.usersByID[userID] = u
	return u, true
}

func (s *MemoryStore) DeleteUser(userID string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	u, ok := s.usersByID[userID]
	if !ok {
		return false
	}
	delete(s.usersByUsername, u.Username)
	delete(s.usersByID, userID)
	return true
}

func (s *MemoryStore) CreateEvent(e domain.Event) (domain.Event, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if e.EventID == "" {
		e.EventID = newID("evt")
	}
	if _, exists := s.events[e.EventID]; exists {
		return domain.Event{}, errors.New("event already exists")
	}
	now := time.Now().UTC()
	s.eventSeq++
	e.ID = s.eventSeq
	if e.Severity == "" {
		e.Severity = "medium"
	}
	if e.Source == "" {
		e.Source = "manual"
	}
	if e.EventStatus == "" {
		e.EventStatus = "pending"
	}
	if e.CurrentRound == 0 {
		e.CurrentRound = 1
	}
	e.CreatedAt = now
	e.UpdatedAt = now
	s.events[e.EventID] = e
	return e, nil
}

func (s *MemoryStore) GetEvent(eventID string) (domain.Event, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	e, ok := s.events[eventID]
	return e, ok
}

func (s *MemoryStore) ListEvents() []domain.Event {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]domain.Event, 0, len(s.events))
	for _, e := range s.events {
		out = append(out, e)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	return out
}

func (s *MemoryStore) UpdateEvent(eventID string, patch map[string]any) (domain.Event, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	e, ok := s.events[eventID]
	if !ok {
		return domain.Event{}, false
	}
	if v, ok := stringPatch(patch, "event_name"); ok {
		e.EventName = v
	}
	if v, ok := stringPatch(patch, "message"); ok {
		e.Message = v
	}
	if v, ok := stringPatch(patch, "context"); ok {
		e.Context = v
	}
	if v, ok := stringPatch(patch, "severity"); ok {
		e.Severity = v
	}
	if v, ok := stringPatch(patch, "event_status"); ok {
		e.EventStatus = v
	}
	e.UpdatedAt = time.Now().UTC()
	s.events[eventID] = e
	return e, true
}

func (s *MemoryStore) AddMessage(m domain.Message) (domain.Message, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.events[m.EventID]; !ok {
		return domain.Message{}, errors.New("event not found")
	}
	s.messageSeq++
	m.ID = s.messageSeq
	if m.MessageID == "" {
		m.MessageID = newID("msg")
	}
	if m.CreatedAt.IsZero() {
		m.CreatedAt = time.Now().UTC()
	}
	s.messagesByEvent[m.EventID] = append(s.messagesByEvent[m.EventID], m)
	return m, nil
}

func (s *MemoryStore) ListMessages(eventID string) []domain.Message {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := append([]domain.Message(nil), s.messagesByEvent[eventID]...)
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

func (s *MemoryStore) ListTasks(eventID string) []domain.Task {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return append([]domain.Task(nil), s.tasksByEvent[eventID]...)
}

func (s *MemoryStore) ListExecutions(eventID string) []domain.Execution {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return append([]domain.Execution(nil), s.execByEvent[eventID]...)
}

func (s *MemoryStore) ListSummaries(eventID string) []domain.Summary {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return append([]domain.Summary(nil), s.summariesByEvent[eventID]...)
}

func (s *MemoryStore) ReserveFingerprint(fp string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.eventMaps[fp]; exists {
		return false
	}
	s.eventMaps[fp] = domain.EventMap{Fingerprint: fp, CreatedAt: time.Now().UTC()}
	return true
}

func (s *MemoryStore) BindEventMap(fp, lyID, deepSOCID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	row := s.eventMaps[fp]
	row.Fingerprint = fp
	row.LyEventID = lyID
	row.DeepSOCEventID = deepSOCID
	if row.CreatedAt.IsZero() {
		row.CreatedAt = time.Now().UTC()
	}
	s.eventMaps[fp] = row
}

func (s *MemoryStore) GetEventMap(fp string) (domain.EventMap, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	row, ok := s.eventMaps[fp]
	return row, ok
}

func (s *MemoryStore) GetCursor(name string) domain.SyncCursor {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if c, ok := s.cursors[name]; ok {
		return c
	}
	return domain.SyncCursor{Name: name}
}

func (s *MemoryStore) SaveCursor(c domain.SyncCursor) {
	s.mu.Lock()
	defer s.mu.Unlock()
	c.UpdatedAt = time.Now().UTC()
	s.cursors[c.Name] = c
}

func (s *MemoryStore) AlreadyPushed(id string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	_, ok := s.pushed[id]
	return ok
}

func (s *MemoryStore) SavePushedEvent(pe domain.PushedEvent) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now().UTC()
	if pe.CreatedAt.IsZero() {
		pe.CreatedAt = now
	}
	pe.UpdatedAt = now
	s.pushed[pe.LyEventID] = pe
}

func (s *MemoryStore) AddAuditLog(a domain.AuditLog) domain.AuditLog {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.auditSeq++
	a.ID = s.auditSeq
	if a.CreatedAt.IsZero() {
		a.CreatedAt = time.Now().UTC()
	}
	s.audits = append(s.audits, a)
	return a
}
