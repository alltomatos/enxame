package domain

import (
	"time"
)

// EventType representa o tipo de evento global
type EventType int

const (
	EventTypeUnspecified EventType = iota
	EventTypeNodeBanned
	EventTypeContentBanned
	EventTypeGlobalAlert
	EventTypeRelayOffline
	EventTypeNetworkUpdate
)

func (t EventType) String() string {
	switch t {
	case EventTypeNodeBanned:
		return "node_banned"
	case EventTypeContentBanned:
		return "content_banned"
	case EventTypeGlobalAlert:
		return "global_alert"
	case EventTypeRelayOffline:
		return "relay_offline"
	case EventTypeNetworkUpdate:
		return "network_update"
	default:
		return "unspecified"
	}
}

// GlobalModerator representa um moderador global da rede
type GlobalModerator struct {
	ModeratorID string
	PublicKey   []byte
	Name        string
	CreatedAt   time.Time
	Active      bool
}

// ModerationAction representa uma ação de moderação
type ModerationAction struct {
	ActionID  string
	Type      EventType
	TargetID  string // NodeID ou ContentHash dependendo do tipo
	Reason    string
	Duration  time.Duration // 0 = permanente
	Moderator *GlobalModerator
	Signature []byte
	CreatedAt time.Time
}

// NodeBan representa um banimento de nó
type NodeBan struct {
	BannedNodeID    string
	Reason          string
	DurationSeconds int64
	EvidenceHash    string
	BannedAt        time.Time
	ExpiresAt       *time.Time // nil = permanente
	ModeratorID     string
}

// ContentBan representa um banimento de conteúdo
type ContentBan struct {
	ContentHash string
	Reason      string
	Category    string
	BannedAt    time.Time
	ModeratorID string
}

// GlobalAlert representa um alerta global
type GlobalAlert struct {
	AlertID   string
	Title     string
	Message   string
	Severity  string // info, warning, critical
	InfoURL   string
	CreatedAt time.Time
}

// ModerationEvent representa um evento de moderação para streaming
type ModerationEvent struct {
	EventID   string
	Type      EventType
	Timestamp time.Time
	Payload   interface{} // NodeBan, ContentBan, GlobalAlert, etc.
	Signature *ModeratorSignature
}

// ModeratorSignature representa a assinatura de um moderador em um evento
type ModeratorSignature struct {
	ModeratorID string
	PublicKey   []byte
	Signature   []byte
	SignedAt    time.Time
}

// IsPermanent verifica se um ban é permanente
func (b *NodeBan) IsPermanent() bool {
	return b.ExpiresAt == nil
}

// IsExpired verifica se um ban expirou
func (b *NodeBan) IsExpired() bool {
	if b.IsPermanent() {
		return false
	}
	return time.Now().After(*b.ExpiresAt)
}
