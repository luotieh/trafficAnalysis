package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	_ "github.com/lib/pq"

	"traffic-go/internal/domain"
)

type PostgresStore struct {
	db *sql.DB
}

func NewPostgresStore(ctx context.Context, databaseURL string, autoMigrate bool) (*PostgresStore, error) {
	if databaseURL == "" {
		return nil, errors.New("DATABASE_URL is empty")
	}
	db, err := sql.Open("postgres", databaseURL)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(20)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(30 * time.Minute)

	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, err
	}
	s := &PostgresStore{db: db}
	if autoMigrate {
		if err := s.Migrate(ctx); err != nil {
			_ = db.Close()
			return nil, err
		}
		if err := s.Seed(ctx); err != nil {
			_ = db.Close()
			return nil, err
		}
	}
	return s, nil
}

func (s *PostgresStore) Migrate(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, PostgresSchema)
	return err
}

func (s *PostgresStore) Seed(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `
INSERT INTO users (user_id, username, nickname, email, password_hash, role, is_active)
VALUES ('admin', 'admin', '管理员', 'admin@example.local', 'admin', 'admin', true)
ON CONFLICT (username) DO NOTHING`)
	return err
}

func (s *PostgresStore) CreateUser(u domain.User) (domain.User, error) {
	if u.UserID == "" {
		u.UserID = newID("u")
	}
	if u.Role == "" {
		u.Role = "user"
	}
	if u.Password == "" {
		u.Password = "ChangeMe123!"
	}
	if u.CreatedAt.IsZero() {
		u.CreatedAt = time.Now().UTC()
	}
	u.UpdatedAt = time.Now().UTC()
	row := s.db.QueryRowContext(context.Background(), `
INSERT INTO users (user_id, username, nickname, email, phone, password_hash, role, is_active, created_at, updated_at)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)
RETURNING id, user_id, username, nickname, email, phone, password_hash, role, last_login_at, is_active, created_at, updated_at`,
		u.UserID, u.Username, u.Nickname, u.Email, u.Phone, u.Password, u.Role, true, u.CreatedAt, u.UpdatedAt)
	return scanUser(row)
}

func (s *PostgresStore) GetUserByUsername(username string) (domain.User, bool) {
	row := s.db.QueryRowContext(context.Background(), `
SELECT id, user_id, username, nickname, email, phone, password_hash, role, last_login_at, is_active, created_at, updated_at
FROM users WHERE username=$1`, username)
	u, err := scanUser(row)
	return u, err == nil
}

func (s *PostgresStore) GetUser(userID string) (domain.User, bool) {
	row := s.db.QueryRowContext(context.Background(), `
SELECT id, user_id, username, nickname, email, phone, password_hash, role, last_login_at, is_active, created_at, updated_at
FROM users WHERE user_id=$1`, userID)
	u, err := scanUser(row)
	return u, err == nil
}

func (s *PostgresStore) ListUsers() []domain.User {
	rows, err := s.db.QueryContext(context.Background(), `
SELECT id, user_id, username, nickname, email, phone, password_hash, role, last_login_at, is_active, created_at, updated_at
FROM users ORDER BY id`)
	if err != nil {
		return nil
	}
	defer rows.Close()

	out := []domain.User{}
	for rows.Next() {
		u, err := scanUser(rows)
		if err == nil {
			out = append(out, u)
		}
	}
	return out
}

func (s *PostgresStore) UpdateUser(userID string, patch map[string]any) (domain.User, bool) {
	u, ok := s.GetUser(userID)
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
	row := s.db.QueryRowContext(context.Background(), `
UPDATE users
SET nickname=$2, email=$3, phone=$4, password_hash=$5, role=$6, last_login_at=$7, is_active=$8, updated_at=$9
WHERE user_id=$1
RETURNING id, user_id, username, nickname, email, phone, password_hash, role, last_login_at, is_active, created_at, updated_at`,
		userID, u.Nickname, u.Email, u.Phone, u.Password, u.Role, u.LastLoginAt, u.IsActive, u.UpdatedAt)
	updated, err := scanUser(row)
	return updated, err == nil
}

func (s *PostgresStore) DeleteUser(userID string) bool {
	res, err := s.db.ExecContext(context.Background(), `DELETE FROM users WHERE user_id=$1`, userID)
	if err != nil {
		return false
	}
	n, _ := res.RowsAffected()
	return n > 0
}

func (s *PostgresStore) CreateEvent(e domain.Event) (domain.Event, error) {
	if e.EventID == "" {
		e.EventID = newID("evt")
	}
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
	now := time.Now().UTC()
	if e.CreatedAt.IsZero() {
		e.CreatedAt = now
	}
	e.UpdatedAt = now
	row := s.db.QueryRowContext(context.Background(), `
INSERT INTO events (event_id, event_name, title, message, context, source, severity, category, event_status, current_round, observables, created_at, updated_at)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11::jsonb,$12,$13)
RETURNING id, event_id, event_name, title, message, context, source, severity, category, event_status, current_round, observables, created_at, updated_at`,
		e.EventID, e.EventName, e.Title, e.Message, e.Context, e.Source, e.Severity, e.Category, e.EventStatus, e.CurrentRound, string(toJSON(e.Observables)), e.CreatedAt, e.UpdatedAt)
	return scanEvent(row)
}

func (s *PostgresStore) GetEvent(eventID string) (domain.Event, bool) {
	row := s.db.QueryRowContext(context.Background(), `
SELECT id, event_id, event_name, title, message, context, source, severity, category, event_status, current_round, observables, created_at, updated_at
FROM events WHERE event_id=$1`, eventID)
	e, err := scanEvent(row)
	return e, err == nil
}

func (s *PostgresStore) ListEvents() []domain.Event {
	rows, err := s.db.QueryContext(context.Background(), `
SELECT id, event_id, event_name, title, message, context, source, severity, category, event_status, current_round, observables, created_at, updated_at
FROM events ORDER BY created_at DESC, id DESC`)
	if err != nil {
		return nil
	}
	defer rows.Close()

	out := []domain.Event{}
	for rows.Next() {
		e, err := scanEvent(rows)
		if err == nil {
			out = append(out, e)
		}
	}
	return out
}

func (s *PostgresStore) UpdateEvent(eventID string, patch map[string]any) (domain.Event, bool) {
	e, ok := s.GetEvent(eventID)
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
	row := s.db.QueryRowContext(context.Background(), `
UPDATE events
SET event_name=$2, title=$3, message=$4, context=$5, source=$6, severity=$7, category=$8, event_status=$9, current_round=$10, observables=$11::jsonb, updated_at=$12
WHERE event_id=$1
RETURNING id, event_id, event_name, title, message, context, source, severity, category, event_status, current_round, observables, created_at, updated_at`,
		eventID, e.EventName, e.Title, e.Message, e.Context, e.Source, e.Severity, e.Category, e.EventStatus, e.CurrentRound, string(toJSON(e.Observables)), e.UpdatedAt)
	updated, err := scanEvent(row)
	return updated, err == nil
}

func (s *PostgresStore) AddMessage(m domain.Message) (domain.Message, error) {
	if m.MessageID == "" {
		m.MessageID = newID("msg")
	}
	if m.RoundID == 0 {
		m.RoundID = 1
	}
	if m.CreatedAt.IsZero() {
		m.CreatedAt = time.Now().UTC()
	}
	row := s.db.QueryRowContext(context.Background(), `
INSERT INTO messages (message_id, event_id, user_id, user_nickname, message_from, message_type, message_content, round_id, created_at)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)
RETURNING id, message_id, event_id, user_id, user_nickname, message_from, message_type, message_content, round_id, created_at`,
		m.MessageID, m.EventID, m.UserID, m.UserNickname, m.MessageFrom, m.MessageType, m.MessageContent, m.RoundID, m.CreatedAt)
	return scanMessage(row)
}

func (s *PostgresStore) ListMessages(eventID string) []domain.Message {
	rows, err := s.db.QueryContext(context.Background(), `
SELECT id, message_id, event_id, user_id, user_nickname, message_from, message_type, message_content, round_id, created_at
FROM messages WHERE event_id=$1 ORDER BY id`, eventID)
	if err != nil {
		return nil
	}
	defer rows.Close()

	out := []domain.Message{}
	for rows.Next() {
		m, err := scanMessage(rows)
		if err == nil {
			out = append(out, m)
		}
	}
	return out
}

func (s *PostgresStore) ListTasks(eventID string) []domain.Task {
	rows, err := s.db.QueryContext(context.Background(), `
SELECT id, task_id, event_id, task_name, task_description, task_status, task_priority, assigned_to, round_id, created_at, updated_at
FROM tasks WHERE event_id=$1 ORDER BY id`, eventID)
	if err != nil {
		return nil
	}
	defer rows.Close()

	out := []domain.Task{}
	for rows.Next() {
		var t domain.Task
		if err := rows.Scan(&t.ID, &t.TaskID, &t.EventID, &t.TaskName, &t.TaskDescription, &t.TaskStatus, &t.TaskPriority, &t.AssignedTo, &t.RoundID, &t.CreatedAt, &t.UpdatedAt); err == nil {
			out = append(out, t)
		}
	}
	return out
}

func (s *PostgresStore) ListExecutions(eventID string) []domain.Execution {
	rows, err := s.db.QueryContext(context.Background(), `
SELECT id, execution_id, event_id, command_id, execution_status, execution_result, command_name, command_type, command_entity, command_params, created_at, updated_at
FROM executions WHERE event_id=$1 ORDER BY id`, eventID)
	if err != nil {
		return nil
	}
	defer rows.Close()

	out := []domain.Execution{}
	for rows.Next() {
		var e domain.Execution
		if err := rows.Scan(&e.ID, &e.ExecutionID, &e.EventID, &e.CommandID, &e.ExecutionStatus, &e.ExecutionResult, &e.CommandName, &e.CommandType, &e.CommandEntity, &e.CommandParams, &e.CreatedAt, &e.UpdatedAt); err == nil {
			out = append(out, e)
		}
	}
	return out
}

func (s *PostgresStore) ListSummaries(eventID string) []domain.Summary {
	rows, err := s.db.QueryContext(context.Background(), `
SELECT id, event_id, round_id, event_summary, created_at, updated_at
FROM summaries WHERE event_id=$1 ORDER BY id`, eventID)
	if err != nil {
		return nil
	}
	defer rows.Close()

	out := []domain.Summary{}
	for rows.Next() {
		var sm domain.Summary
		if err := rows.Scan(&sm.ID, &sm.EventID, &sm.RoundID, &sm.EventSummary, &sm.CreatedAt, &sm.UpdatedAt); err == nil {
			out = append(out, sm)
		}
	}
	return out
}

func (s *PostgresStore) ReserveFingerprint(fp string) bool {
	res, err := s.db.ExecContext(context.Background(), `
INSERT INTO event_maps (fingerprint) VALUES ($1)
ON CONFLICT (fingerprint) DO NOTHING`, fp)
	if err != nil {
		return false
	}
	n, _ := res.RowsAffected()
	return n > 0
}

func (s *PostgresStore) BindEventMap(fp, lyID, deepSOCID string) {
	_, _ = s.db.ExecContext(context.Background(), `
UPDATE event_maps SET ly_event_id=$2, deepsoc_event_id=$3 WHERE fingerprint=$1`,
		fp, lyID, deepSOCID)
}

func (s *PostgresStore) GetEventMap(fp string) (domain.EventMap, bool) {
	var row domain.EventMap
	err := s.db.QueryRowContext(context.Background(), `
SELECT fingerprint, ly_event_id, deepsoc_event_id, created_at FROM event_maps WHERE fingerprint=$1`, fp).
		Scan(&row.Fingerprint, &row.LyEventID, &row.DeepSOCEventID, &row.CreatedAt)
	return row, err == nil
}

func (s *PostgresStore) GetCursor(name string) domain.SyncCursor {
	var c domain.SyncCursor
	err := s.db.QueryRowContext(context.Background(), `
SELECT name, last_ts, updated_at FROM sync_cursors WHERE name=$1`, name).
		Scan(&c.Name, &c.LastTS, &c.UpdatedAt)
	if err != nil {
		return domain.SyncCursor{Name: name}
	}
	return c
}

func (s *PostgresStore) SaveCursor(c domain.SyncCursor) {
	if c.UpdatedAt.IsZero() {
		c.UpdatedAt = time.Now().UTC()
	}
	_, _ = s.db.ExecContext(context.Background(), `
INSERT INTO sync_cursors (name, last_ts, updated_at)
VALUES ($1,$2,$3)
ON CONFLICT (name) DO UPDATE SET last_ts=EXCLUDED.last_ts, updated_at=EXCLUDED.updated_at`,
		c.Name, c.LastTS, c.UpdatedAt)
}

func (s *PostgresStore) AlreadyPushed(id string) bool {
	var n int
	err := s.db.QueryRowContext(context.Background(), `SELECT 1 FROM pushed_events WHERE ly_event_id=$1`, id).Scan(&n)
	return err == nil
}

func (s *PostgresStore) SavePushedEvent(pe domain.PushedEvent) {
	now := time.Now().UTC()
	if pe.CreatedAt.IsZero() {
		pe.CreatedAt = now
	}
	pe.UpdatedAt = now
	_, _ = s.db.ExecContext(context.Background(), `
INSERT INTO pushed_events (ly_event_id, idempotency_key, deepsoc_event_id, status, attempts, last_error, created_at, updated_at)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
ON CONFLICT (ly_event_id) DO UPDATE SET
idempotency_key=EXCLUDED.idempotency_key,
deepsoc_event_id=EXCLUDED.deepsoc_event_id,
status=EXCLUDED.status,
attempts=EXCLUDED.attempts,
last_error=EXCLUDED.last_error,
updated_at=EXCLUDED.updated_at`,
		pe.LyEventID, pe.IdempotencyKey, pe.DeepSOCEventID, pe.Status, pe.Attempts, pe.LastError, pe.CreatedAt, pe.UpdatedAt)
}

func (s *PostgresStore) AddAuditLog(a domain.AuditLog) domain.AuditLog {
	if a.CreatedAt.IsZero() {
		a.CreatedAt = time.Now().UTC()
	}
	_ = s.db.QueryRowContext(context.Background(), `
INSERT INTO audit_logs (actor, action, target, meta, created_at)
VALUES ($1,$2,$3,$4,$5)
RETURNING id`, a.Actor, a.Action, a.Target, a.Meta, a.CreatedAt).Scan(&a.ID)
	return a
}

type scanner interface {
	Scan(dest ...any) error
}

func scanUser(row scanner) (domain.User, error) {
	var u domain.User
	var last sql.NullTime
	err := row.Scan(&u.ID, &u.UserID, &u.Username, &u.Nickname, &u.Email, &u.Phone, &u.Password, &u.Role, &last, &u.IsActive, &u.CreatedAt, &u.UpdatedAt)
	if last.Valid {
		u.LastLoginAt = &last.Time
	}
	return u, err
}

func scanEvent(row scanner) (domain.Event, error) {
	var e domain.Event
	var obs []byte
	err := row.Scan(&e.ID, &e.EventID, &e.EventName, &e.Title, &e.Message, &e.Context, &e.Source, &e.Severity, &e.Category, &e.EventStatus, &e.CurrentRound, &obs, &e.CreatedAt, &e.UpdatedAt)
	if len(obs) > 0 {
		_ = json.Unmarshal(obs, &e.Observables)
	}
	return e, err
}

func scanMessage(row scanner) (domain.Message, error) {
	var m domain.Message
	err := row.Scan(&m.ID, &m.MessageID, &m.EventID, &m.UserID, &m.UserNickname, &m.MessageFrom, &m.MessageType, &m.MessageContent, &m.RoundID, &m.CreatedAt)
	return m, err
}
