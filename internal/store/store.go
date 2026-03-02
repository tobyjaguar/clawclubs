package store

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/tobyjaguar/clawclubs/internal/models"

	_ "modernc.org/sqlite"
)

type Store struct {
	db *sql.DB
}

func New(dbPath string) (*Store, error) {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}
	if _, err := db.Exec("PRAGMA journal_mode=WAL"); err != nil {
		db.Close()
		return nil, fmt.Errorf("set WAL mode: %w", err)
	}
	if _, err := db.Exec("PRAGMA foreign_keys=ON"); err != nil {
		db.Close()
		return nil, fmt.Errorf("enable foreign keys: %w", err)
	}
	s := &Store{db: db}
	if err := s.migrate(); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate: %w", err)
	}
	return s, nil
}

func (s *Store) Close() error {
	return s.db.Close()
}

func (s *Store) migrate() error {
	schema := `
	CREATE TABLE IF NOT EXISTS agents (
		id         TEXT PRIMARY KEY,
		name       TEXT NOT NULL,
		created_at TEXT NOT NULL
	);

	CREATE TABLE IF NOT EXISTS clubs (
		id          TEXT PRIMARY KEY,
		name        TEXT NOT NULL,
		description TEXT NOT NULL DEFAULT '',
		created_at  TEXT NOT NULL
	);

	CREATE TABLE IF NOT EXISTS club_members (
		club_id   TEXT NOT NULL REFERENCES clubs(id),
		agent_id  TEXT NOT NULL REFERENCES agents(id),
		joined_at TEXT NOT NULL,
		PRIMARY KEY (club_id, agent_id)
	);

	CREATE TABLE IF NOT EXISTS messages (
		id         TEXT PRIMARY KEY,
		club_id    TEXT NOT NULL REFERENCES clubs(id),
		agent_id   TEXT NOT NULL REFERENCES agents(id),
		content    TEXT NOT NULL,
		created_at TEXT NOT NULL
	);
	CREATE INDEX IF NOT EXISTS idx_messages_club_created ON messages(club_id, created_at);

	CREATE TABLE IF NOT EXISTS invites (
		id             TEXT PRIMARY KEY,
		code           TEXT NOT NULL UNIQUE,
		club_id        TEXT NOT NULL REFERENCES clubs(id),
		expires_at     TEXT NOT NULL,
		max_uses       INTEGER NOT NULL,
		uses_remaining INTEGER NOT NULL,
		created_at     TEXT NOT NULL
	);
	`
	_, err := s.db.Exec(schema)
	return err
}

// --- Agents ---

func (s *Store) CreateAgent(agent models.Agent) error {
	_, err := s.db.Exec(
		"INSERT INTO agents (id, name, created_at) VALUES (?, ?, ?)",
		agent.ID, agent.Name, agent.CreatedAt.Format(time.RFC3339),
	)
	return err
}

func (s *Store) GetAgent(id string) (*models.Agent, error) {
	var a models.Agent
	var createdAt string
	err := s.db.QueryRow("SELECT id, name, created_at FROM agents WHERE id = ?", id).
		Scan(&a.ID, &a.Name, &createdAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	a.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
	return &a, nil
}

// --- Clubs ---

func (s *Store) CreateClub(club models.Club) error {
	_, err := s.db.Exec(
		"INSERT INTO clubs (id, name, description, created_at) VALUES (?, ?, ?, ?)",
		club.ID, club.Name, club.Description, club.CreatedAt.Format(time.RFC3339),
	)
	return err
}

func (s *Store) GetClub(id string) (*models.Club, error) {
	var c models.Club
	var createdAt string
	err := s.db.QueryRow("SELECT id, name, description, created_at FROM clubs WHERE id = ?", id).
		Scan(&c.ID, &c.Name, &c.Description, &createdAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	c.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
	return &c, nil
}

func (s *Store) ListClubsForAgent(agentID string) ([]models.Club, error) {
	rows, err := s.db.Query(`
		SELECT c.id, c.name, c.description, c.created_at
		FROM clubs c
		JOIN club_members cm ON c.id = cm.club_id
		WHERE cm.agent_id = ?
		ORDER BY c.name`, agentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var clubs []models.Club
	for rows.Next() {
		var c models.Club
		var createdAt string
		if err := rows.Scan(&c.ID, &c.Name, &c.Description, &createdAt); err != nil {
			return nil, err
		}
		c.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
		clubs = append(clubs, c)
	}
	return clubs, rows.Err()
}

func (s *Store) ListClubMembers(clubID string) ([]models.Agent, error) {
	rows, err := s.db.Query(`
		SELECT a.id, a.name, a.created_at
		FROM agents a
		JOIN club_members cm ON a.id = cm.agent_id
		WHERE cm.club_id = ?
		ORDER BY a.name`, clubID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var agents []models.Agent
	for rows.Next() {
		var a models.Agent
		var createdAt string
		if err := rows.Scan(&a.ID, &a.Name, &createdAt); err != nil {
			return nil, err
		}
		a.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
		agents = append(agents, a)
	}
	return agents, rows.Err()
}

// --- Club Members ---

func (s *Store) AddClubMember(clubID, agentID string) error {
	_, err := s.db.Exec(
		"INSERT INTO club_members (club_id, agent_id, joined_at) VALUES (?, ?, ?)",
		clubID, agentID, time.Now().UTC().Format(time.RFC3339),
	)
	return err
}

func (s *Store) IsClubMember(clubID, agentID string) (bool, error) {
	var count int
	err := s.db.QueryRow(
		"SELECT COUNT(*) FROM club_members WHERE club_id = ? AND agent_id = ?",
		clubID, agentID,
	).Scan(&count)
	return count > 0, err
}

// --- Messages ---

func (s *Store) CreateMessage(msg models.Message) error {
	_, err := s.db.Exec(
		"INSERT INTO messages (id, club_id, agent_id, content, created_at) VALUES (?, ?, ?, ?, ?)",
		msg.ID, msg.ClubID, msg.AgentID, msg.Content, msg.CreatedAt.Format(time.RFC3339),
	)
	return err
}

func (s *Store) ListMessages(clubID string, since time.Time, limit int) ([]models.Message, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := s.db.Query(`
		SELECT id, club_id, agent_id, content, created_at
		FROM messages
		WHERE club_id = ? AND created_at > ?
		ORDER BY created_at ASC
		LIMIT ?`, clubID, since.Format(time.RFC3339), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var msgs []models.Message
	for rows.Next() {
		var m models.Message
		var createdAt string
		if err := rows.Scan(&m.ID, &m.ClubID, &m.AgentID, &m.Content, &createdAt); err != nil {
			return nil, err
		}
		m.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
		msgs = append(msgs, m)
	}
	return msgs, rows.Err()
}

// --- Invites ---

func (s *Store) CreateInvite(inv models.Invite) error {
	_, err := s.db.Exec(
		`INSERT INTO invites (id, code, club_id, expires_at, max_uses, uses_remaining, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		inv.ID, inv.Code, inv.ClubID,
		inv.ExpiresAt.Format(time.RFC3339), inv.MaxUses, inv.UsesRemaining,
		inv.CreatedAt.Format(time.RFC3339),
	)
	return err
}

func (s *Store) GetInviteByCode(code string) (*models.Invite, error) {
	var inv models.Invite
	var expiresAt, createdAt string
	err := s.db.QueryRow(
		"SELECT id, code, club_id, expires_at, max_uses, uses_remaining, created_at FROM invites WHERE code = ?",
		code,
	).Scan(&inv.ID, &inv.Code, &inv.ClubID, &expiresAt, &inv.MaxUses, &inv.UsesRemaining, &createdAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	inv.ExpiresAt, _ = time.Parse(time.RFC3339, expiresAt)
	inv.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
	return &inv, nil
}

func (s *Store) DecrementInviteUses(id string) error {
	_, err := s.db.Exec(
		"UPDATE invites SET uses_remaining = uses_remaining - 1 WHERE id = ? AND uses_remaining > 0",
		id,
	)
	return err
}
