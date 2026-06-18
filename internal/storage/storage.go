// Package storage persists conversation state and notification subscriptions
// in a SQLite database.
package storage

import (
	"database/sql"
	"fmt"

	_ "modernc.org/sqlite"
)

// Conversation steps used by the /add wizard.
const (
	StepIdle             = ""
	StepAwaitingBus      = "awaiting_bus"
	StepAwaitingStopText = "awaiting_stop_text"
	StepAwaitingStopPick = "awaiting_stop_pick"
	StepAwaitingReminder = "awaiting_reminder"
	StepAwaitingStart    = "awaiting_start"
	StepAwaitingEnd      = "awaiting_end"
	StepAwaitingName     = "awaiting_name"
)

// StopChoice is a candidate stop offered to the user during the /add wizard.
// A slice of these is JSON-encoded into State.StopResults.
type StopChoice struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Code string `json:"code"`
}

// State holds the in-progress wizard state for a user.
type State struct {
	UserID          int64
	ChatID          int64
	Step            string
	BusNumber       string
	StopID          string
	StopName        string
	StopCode        string
	StopResults     string // JSON-encoded []StopChoice
	ReminderMinutes int
	StartMinutes    int
	EndMinutes      int
	Name            string
}

// Notification is a saved arrival-reminder subscription.
type Notification struct {
	ID              int64
	UserID          int64
	ChatID          int64
	Name            string
	BusNumber       string
	StopID          string
	StopName        string
	StopCode        string
	ReminderMinutes int
	StartMinutes    int
	EndMinutes      int
	LastNotifiedAt  sql.NullInt64 // unix seconds
}

// Store wraps a SQLite database connection.
type Store struct {
	db *sql.DB
}

// Open opens (creating if necessary) the SQLite database at path and applies
// the schema.
func Open(path string) (*Store, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("storage: open: %w", err)
	}
	// SQLite handles concurrency best with a single writer connection.
	db.SetMaxOpenConns(1)

	s := &Store{db: db}
	if err := s.migrate(); err != nil {
		db.Close()
		return nil, err
	}
	return s, nil
}

// Close closes the underlying database.
func (s *Store) Close() error { return s.db.Close() }

func (s *Store) migrate() error {
	const schema = `
PRAGMA journal_mode = WAL;

CREATE TABLE IF NOT EXISTS states (
	user_id          INTEGER PRIMARY KEY,
	chat_id          INTEGER NOT NULL,
	step             TEXT    NOT NULL DEFAULT '',
	bus_number       TEXT    NOT NULL DEFAULT '',
	stop_id          TEXT    NOT NULL DEFAULT '',
	stop_name        TEXT    NOT NULL DEFAULT '',
	stop_code        TEXT    NOT NULL DEFAULT '',
	stop_results     TEXT    NOT NULL DEFAULT '',
	reminder_minutes INTEGER NOT NULL DEFAULT 0,
	start_minutes    INTEGER NOT NULL DEFAULT 0,
	end_minutes      INTEGER NOT NULL DEFAULT 0,
	name             TEXT    NOT NULL DEFAULT ''
);

CREATE TABLE IF NOT EXISTS notifications (
	id               INTEGER PRIMARY KEY AUTOINCREMENT,
	user_id          INTEGER NOT NULL,
	chat_id          INTEGER NOT NULL,
	name             TEXT    NOT NULL,
	bus_number       TEXT    NOT NULL,
	stop_id          TEXT    NOT NULL,
	stop_name        TEXT    NOT NULL,
	stop_code        TEXT    NOT NULL DEFAULT '',
	reminder_minutes INTEGER NOT NULL,
	start_minutes    INTEGER NOT NULL,
	end_minutes      INTEGER NOT NULL,
	last_notified_at INTEGER
);

CREATE INDEX IF NOT EXISTS idx_notifications_user ON notifications(user_id);
`
	if _, err := s.db.Exec(schema); err != nil {
		return fmt.Errorf("storage: migrate: %w", err)
	}

	// Add columns that were introduced after the original schema. Existing
	// databases won't have them; new ones already do via CREATE TABLE above.
	s.addColumn("states", "stop_code", "TEXT NOT NULL DEFAULT ''")
	s.addColumn("notifications", "stop_code", "TEXT NOT NULL DEFAULT ''")
	return nil
}

// addColumn adds a column if it doesn't already exist, ignoring the
// "duplicate column" error so it is safe to run on every startup.
func (s *Store) addColumn(table, column, definition string) {
	_, _ = s.db.Exec(fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s %s", table, column, definition))
}

// GetState returns the wizard state for a user, or nil if none exists.
func (s *Store) GetState(userID int64) (*State, error) {
	row := s.db.QueryRow(`
SELECT user_id, chat_id, step, bus_number, stop_id, stop_name, stop_code, stop_results,
       reminder_minutes, start_minutes, end_minutes, name
FROM states WHERE user_id = ?`, userID)

	var st State
	err := row.Scan(&st.UserID, &st.ChatID, &st.Step, &st.BusNumber, &st.StopID,
		&st.StopName, &st.StopCode, &st.StopResults, &st.ReminderMinutes, &st.StartMinutes, &st.EndMinutes, &st.Name)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("storage: get state: %w", err)
	}
	return &st, nil
}

// SaveState inserts or updates the wizard state for a user.
func (s *Store) SaveState(st *State) error {
	_, err := s.db.Exec(`
INSERT INTO states (user_id, chat_id, step, bus_number, stop_id, stop_name, stop_code,
                    stop_results, reminder_minutes, start_minutes, end_minutes, name)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(user_id) DO UPDATE SET
	chat_id = excluded.chat_id,
	step = excluded.step,
	bus_number = excluded.bus_number,
	stop_id = excluded.stop_id,
	stop_name = excluded.stop_name,
	stop_code = excluded.stop_code,
	stop_results = excluded.stop_results,
	reminder_minutes = excluded.reminder_minutes,
	start_minutes = excluded.start_minutes,
	end_minutes = excluded.end_minutes,
	name = excluded.name`,
		st.UserID, st.ChatID, st.Step, st.BusNumber, st.StopID, st.StopName, st.StopCode,
		st.StopResults, st.ReminderMinutes, st.StartMinutes, st.EndMinutes, st.Name)
	if err != nil {
		return fmt.Errorf("storage: save state: %w", err)
	}
	return nil
}

// ClearState removes any wizard state for a user.
func (s *Store) ClearState(userID int64) error {
	_, err := s.db.Exec(`DELETE FROM states WHERE user_id = ?`, userID)
	if err != nil {
		return fmt.Errorf("storage: clear state: %w", err)
	}
	return nil
}

// AddNotification stores a new notification and returns its generated ID.
func (s *Store) AddNotification(n *Notification) (int64, error) {
	res, err := s.db.Exec(`
INSERT INTO notifications (user_id, chat_id, name, bus_number, stop_id, stop_name, stop_code,
                          reminder_minutes, start_minutes, end_minutes)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		n.UserID, n.ChatID, n.Name, n.BusNumber, n.StopID, n.StopName, n.StopCode,
		n.ReminderMinutes, n.StartMinutes, n.EndMinutes)
	if err != nil {
		return 0, fmt.Errorf("storage: add notification: %w", err)
	}
	return res.LastInsertId()
}

// ListNotifications returns all notifications belonging to a user.
func (s *Store) ListNotifications(userID int64) ([]Notification, error) {
	rows, err := s.db.Query(`
SELECT id, user_id, chat_id, name, bus_number, stop_id, stop_name, stop_code,
       reminder_minutes, start_minutes, end_minutes, last_notified_at
FROM notifications WHERE user_id = ? ORDER BY id`, userID)
	if err != nil {
		return nil, fmt.Errorf("storage: list notifications: %w", err)
	}
	defer rows.Close()
	return scanNotifications(rows)
}

// AllNotifications returns every stored notification (used by the notifier).
func (s *Store) AllNotifications() ([]Notification, error) {
	rows, err := s.db.Query(`
SELECT id, user_id, chat_id, name, bus_number, stop_id, stop_name, stop_code,
       reminder_minutes, start_minutes, end_minutes, last_notified_at
FROM notifications ORDER BY id`)
	if err != nil {
		return nil, fmt.Errorf("storage: all notifications: %w", err)
	}
	defer rows.Close()
	return scanNotifications(rows)
}

// GetNotification returns a single notification owned by the given user, or nil.
func (s *Store) GetNotification(id, userID int64) (*Notification, error) {
	row := s.db.QueryRow(`
SELECT id, user_id, chat_id, name, bus_number, stop_id, stop_name, stop_code,
       reminder_minutes, start_minutes, end_minutes, last_notified_at
FROM notifications WHERE id = ? AND user_id = ?`, id, userID)

	var n Notification
	err := row.Scan(&n.ID, &n.UserID, &n.ChatID, &n.Name, &n.BusNumber, &n.StopID,
		&n.StopName, &n.StopCode, &n.ReminderMinutes, &n.StartMinutes, &n.EndMinutes, &n.LastNotifiedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("storage: get notification: %w", err)
	}
	return &n, nil
}

// DeleteNotification removes a notification owned by the given user. It returns
// true if a row was actually deleted.
func (s *Store) DeleteNotification(id, userID int64) (bool, error) {
	res, err := s.db.Exec(`DELETE FROM notifications WHERE id = ? AND user_id = ?`, id, userID)
	if err != nil {
		return false, fmt.Errorf("storage: delete notification: %w", err)
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

// SetLastNotified records the time a reminder was last sent for a notification.
func (s *Store) SetLastNotified(id, unix int64) error {
	_, err := s.db.Exec(`UPDATE notifications SET last_notified_at = ? WHERE id = ?`, unix, id)
	if err != nil {
		return fmt.Errorf("storage: set last notified: %w", err)
	}
	return nil
}

func scanNotifications(rows *sql.Rows) ([]Notification, error) {
	var out []Notification
	for rows.Next() {
		var n Notification
		if err := rows.Scan(&n.ID, &n.UserID, &n.ChatID, &n.Name, &n.BusNumber,
			&n.StopID, &n.StopName, &n.StopCode, &n.ReminderMinutes, &n.StartMinutes,
			&n.EndMinutes, &n.LastNotifiedAt); err != nil {
			return nil, fmt.Errorf("storage: scan notification: %w", err)
		}
		out = append(out, n)
	}
	return out, rows.Err()
}
