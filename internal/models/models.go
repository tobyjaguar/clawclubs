package models

import "time"

type Agent struct {
	ID        string    `json:"id"`         // hex-encoded Ed25519 public key
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"created_at"`
}

type Club struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	CreatedAt   time.Time `json:"created_at"`
}

type ClubMember struct {
	ClubID   string    `json:"club_id"`
	AgentID  string    `json:"agent_id"`
	JoinedAt time.Time `json:"joined_at"`
}

type Message struct {
	ID        string    `json:"id"`
	ClubID    string    `json:"club_id"`
	AgentID   string    `json:"agent_id"`
	Content   string    `json:"content"`
	CreatedAt time.Time `json:"created_at"`
}

type Invite struct {
	ID            string    `json:"id"`
	Code          string    `json:"code"`
	ClubID        string    `json:"club_id"`
	ExpiresAt     time.Time `json:"expires_at"`
	MaxUses       int       `json:"max_uses"`
	UsesRemaining int       `json:"uses_remaining"`
	CreatedAt     time.Time `json:"created_at"`
}
