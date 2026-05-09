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
	actionsByEvent   map[string][]domain.Action
	commandsByEvent  map[string][]domain.Command
	execByEvent      map[string][]domain.Execution
	summariesByEvent map[string][]domain.Summary
	taskSeq          int64
	actionSeq        int64
	commandSeq       int64
	execSeq          int64
	summarySeq       int64
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
		actionsByEvent:   map[string][]domain.Action{},
		commandsByEvent:  map[string][]domain.Command{},
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
	if m.RoundID == 0 {
		m.RoundID = 1
	}
	m = domain.NormalizeMessage(m)
	s.messagesByEvent[m.EventID] = append(s.messagesByEvent[m.EventID], m)
	return m, nil
}

func (s *MemoryStore) ListMessages(eventID string) []domain.Message {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := append([]domain.Message(nil), s.messagesByEvent[eventID]...)
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	filtered := out[:0]
	for _, m := range out {
		m = domain.NormalizeMessage(m)
		if !domain.IsInternalMessage(m) {
			filtered = append(filtered, m)
		}
	}
	return filtered
}

func (s *MemoryStore) ListTasks(eventID string) []domain.Task {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return append([]domain.Task(nil), s.tasksByEvent[eventID]...)
}

func (s *MemoryStore) AddTask(t domain.Task) (domain.Task, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.events[t.EventID]; !ok {
		return domain.Task{}, errors.New("event not found")
	}
	now := time.Now().UTC()
	s.taskSeq++
	t.ID = s.taskSeq
	if t.TaskID == "" {
		t.TaskID = newID("task")
	}
	if t.TaskStatus == "" {
		t.TaskStatus = "pending"
	}
	if t.RoundID == 0 {
		t.RoundID = 1
	}
	if t.CreatedAt.IsZero() {
		t.CreatedAt = now
	}
	t.UpdatedAt = now
	s.tasksByEvent[t.EventID] = append(s.tasksByEvent[t.EventID], t)
	return t, nil
}

func (s *MemoryStore) UpdateTask(taskID string, patch map[string]any) (domain.Task, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for eventID, rows := range s.tasksByEvent {
		for i := range rows {
			if rows[i].TaskID != taskID {
				continue
			}
			if v, ok := stringPatch(patch, "task_status"); ok {
				rows[i].TaskStatus = v
			}
			if v, ok := stringPatch(patch, "assigned_to"); ok {
				rows[i].AssignedTo = v
			}
			rows[i].UpdatedAt = time.Now().UTC()
			s.tasksByEvent[eventID] = rows
			return rows[i], true
		}
	}
	return domain.Task{}, false
}

func (s *MemoryStore) AddAction(a domain.Action) (domain.Action, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.events[a.EventID]; !ok {
		return domain.Action{}, errors.New("event not found")
	}
	now := time.Now().UTC()
	s.actionSeq++
	a.ID = s.actionSeq
	if a.ActionID == "" {
		a.ActionID = newID("act")
	}
	if a.ActionStatus == "" {
		a.ActionStatus = "pending"
	}
	if a.RoundID == 0 {
		a.RoundID = 1
	}
	if a.CreatedAt.IsZero() {
		a.CreatedAt = now
	}
	a.UpdatedAt = now
	s.actionsByEvent[a.EventID] = append(s.actionsByEvent[a.EventID], a)
	return a, nil
}

func (s *MemoryStore) UpdateAction(actionID string, patch map[string]any) (domain.Action, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for eventID, rows := range s.actionsByEvent {
		for i := range rows {
			if rows[i].ActionID != actionID {
				continue
			}
			if v, ok := stringPatch(patch, "action_status"); ok {
				rows[i].ActionStatus = v
			}
			if v, ok := stringPatch(patch, "action_result"); ok {
				rows[i].ActionResult = v
			}
			rows[i].UpdatedAt = time.Now().UTC()
			s.actionsByEvent[eventID] = rows
			return rows[i], true
		}
	}
	return domain.Action{}, false
}

func (s *MemoryStore) ListActions(eventID string) []domain.Action {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return append([]domain.Action(nil), s.actionsByEvent[eventID]...)
}

func (s *MemoryStore) AddCommand(c domain.Command) (domain.Command, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.events[c.EventID]; !ok {
		return domain.Command{}, errors.New("event not found")
	}
	now := time.Now().UTC()
	s.commandSeq++
	c.ID = s.commandSeq
	if c.CommandID == "" {
		c.CommandID = newID("cmd")
	}
	if c.CommandStatus == "" {
		c.CommandStatus = "pending"
	}
	if c.RoundID == 0 {
		c.RoundID = 1
	}
	if c.CreatedAt.IsZero() {
		c.CreatedAt = now
	}
	c.UpdatedAt = now
	s.commandsByEvent[c.EventID] = append(s.commandsByEvent[c.EventID], c)
	return c, nil
}

func (s *MemoryStore) UpdateCommand(commandID string, patch map[string]any) (domain.Command, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for eventID, rows := range s.commandsByEvent {
		for i := range rows {
			if rows[i].CommandID != commandID {
				continue
			}
			if v, ok := stringPatch(patch, "command_status"); ok {
				rows[i].CommandStatus = v
			}
			if v, ok := stringPatch(patch, "command_result"); ok {
				rows[i].CommandResult = v
			}
			rows[i].UpdatedAt = time.Now().UTC()
			s.commandsByEvent[eventID] = rows
			return rows[i], true
		}
	}
	return domain.Command{}, false
}

func (s *MemoryStore) ListCommands(eventID string) []domain.Command {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return append([]domain.Command(nil), s.commandsByEvent[eventID]...)
}

func (s *MemoryStore) AddExecution(e domain.Execution) (domain.Execution, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.events[e.EventID]; !ok {
		return domain.Execution{}, errors.New("event not found")
	}
	now := time.Now().UTC()
	s.execSeq++
	e.ID = s.execSeq
	if e.ExecutionID == "" {
		e.ExecutionID = newID("exec")
	}
	if e.ExecutionStatus == "" {
		e.ExecutionStatus = "pending"
	}
	if e.RoundID == 0 {
		e.RoundID = 1
	}
	if e.CreatedAt.IsZero() {
		e.CreatedAt = now
	}
	e.UpdatedAt = now
	s.execByEvent[e.EventID] = append(s.execByEvent[e.EventID], e)
	return e, nil
}

func (s *MemoryStore) ListExecutions(eventID string) []domain.Execution {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return append([]domain.Execution(nil), s.execByEvent[eventID]...)
}

func (s *MemoryStore) AddSummary(sm domain.Summary) (domain.Summary, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.events[sm.EventID]; !ok {
		return domain.Summary{}, errors.New("event not found")
	}
	now := time.Now().UTC()
	s.summarySeq++
	sm.ID = s.summarySeq
	if sm.RoundID == 0 {
		sm.RoundID = 1
	}
	if sm.CreatedAt.IsZero() {
		sm.CreatedAt = now
	}
	sm.UpdatedAt = now
	s.summariesByEvent[sm.EventID] = append(s.summariesByEvent[sm.EventID], sm)
	return sm, nil
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
