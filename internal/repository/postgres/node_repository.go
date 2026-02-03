package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/goautomatik/core-server/internal/domain"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// NodeRepository persiste metadados de nós no PostgreSQL
type NodeRepository struct {
	pool *pgxpool.Pool
}

// NewNodeRepository cria um novo repositório de nós
func NewNodeRepository(pool *pgxpool.Pool) *NodeRepository {
	return &NodeRepository{pool: pool}
}

// CreateTables cria as tabelas necessárias se não existirem
func (r *NodeRepository) CreateTables(ctx context.Context) error {
	queries := []string{
		`CREATE TABLE IF NOT EXISTS nodes (
			node_id VARCHAR(64) PRIMARY KEY,
			public_key BYTEA NOT NULL,
			type INTEGER NOT NULL,
			version VARCHAR(32),
			region VARCHAR(32),
			capabilities TEXT[],
			first_seen_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
			last_seen_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
			total_uptime_seconds BIGINT DEFAULT 0,
			banned BOOLEAN DEFAULT FALSE,
			ban_reason TEXT,
			ban_expires_at TIMESTAMP WITH TIME ZONE
		)`,
		`CREATE INDEX IF NOT EXISTS idx_nodes_type ON nodes(type)`,
		`CREATE INDEX IF NOT EXISTS idx_nodes_region ON nodes(region)`,
		`CREATE INDEX IF NOT EXISTS idx_nodes_banned ON nodes(banned)`,

		`CREATE TABLE IF NOT EXISTS global_moderators (
			moderator_id VARCHAR(64) PRIMARY KEY,
			public_key BYTEA NOT NULL UNIQUE,
			name VARCHAR(128),
			created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
			active BOOLEAN DEFAULT TRUE
		)`,

		`CREATE TABLE IF NOT EXISTS moderation_actions (
			action_id VARCHAR(64) PRIMARY KEY,
			event_type INTEGER NOT NULL,
			target_id VARCHAR(128) NOT NULL,
			reason TEXT,
			duration_seconds BIGINT,
			moderator_id VARCHAR(64) REFERENCES global_moderators(moderator_id),
			signature BYTEA NOT NULL,
			created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
		)`,
		`CREATE INDEX IF NOT EXISTS idx_moderation_actions_target ON moderation_actions(target_id)`,
		`CREATE INDEX IF NOT EXISTS idx_moderation_actions_type ON moderation_actions(event_type)`,

		`CREATE TABLE IF NOT EXISTS content_bans (
			content_hash VARCHAR(128) PRIMARY KEY,
			reason TEXT,
			category VARCHAR(64),
			banned_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
			moderator_id VARCHAR(64) REFERENCES global_moderators(moderator_id)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_content_bans_category ON content_bans(category)`,

		`CREATE TABLE IF NOT EXISTS governance_audit_log (
			log_id SERIAL PRIMARY KEY,
			timestamp BIGINT NOT NULL,
			requester_id VARCHAR(64) NOT NULL,
			action VARCHAR(32) NOT NULL,
			target_id VARCHAR(64) NOT NULL,
			channel_id VARCHAR(64),
			signature BYTEA NOT NULL,
			created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
		)`,
		`CREATE INDEX IF NOT EXISTS idx_gov_log_requester ON governance_audit_log(requester_id)`,
		`CREATE INDEX IF NOT EXISTS idx_gov_log_channel ON governance_audit_log(channel_id)`,

		// Cluster Peers (High Availability)
		`CREATE TABLE IF NOT EXISTS cluster_peers (
			peer_id VARCHAR(64) PRIMARY KEY,
			address VARCHAR(256) NOT NULL,
			public_key BYTEA NOT NULL,
			status VARCHAR(16) DEFAULT 'pending',
			last_seen TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
			created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
		)`,
		`CREATE INDEX IF NOT EXISTS idx_cluster_peers_status ON cluster_peers(status)`,
	}

	for _, query := range queries {
		if _, err := r.pool.Exec(ctx, query); err != nil {
			return fmt.Errorf("failed to create tables: %w", err)
		}
	}

	return nil
}

// UpsertNode insere ou atualiza um nó
func (r *NodeRepository) UpsertNode(ctx context.Context, node *domain.Node) error {
	query := `
		INSERT INTO nodes (node_id, public_key, type, version, region, capabilities, first_seen_at, last_seen_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		ON CONFLICT (node_id) DO UPDATE SET
			last_seen_at = EXCLUDED.last_seen_at,
			version = EXCLUDED.version,
			region = EXCLUDED.region,
			capabilities = EXCLUDED.capabilities
	`

	_, err := r.pool.Exec(ctx, query,
		node.Identity.NodeID,
		node.Identity.PublicKey,
		int(node.Type),
		node.Version,
		node.Region,
		node.Capabilities,
		node.RegisteredAt,
		time.Now(),
	)

	return err
}

// GetNode obtém um nó pelo ID
func (r *NodeRepository) GetNode(ctx context.Context, nodeID string) (*domain.Node, error) {
	query := `
		SELECT node_id, public_key, type, version, region, capabilities, first_seen_at, last_seen_at, banned, ban_reason, ban_expires_at
		FROM nodes
		WHERE node_id = $1
	`

	var (
		node       domain.Node
		publicKey  []byte
		nodeType   int
		banExpires *time.Time
		banReason  *string
	)

	err := r.pool.QueryRow(ctx, query, nodeID).Scan(
		&node.Identity.NodeID,
		&publicKey,
		&nodeType,
		&node.Version,
		&node.Region,
		&node.Capabilities,
		&node.RegisteredAt,
		&node.LastSeen,
		new(bool), // banned
		&banReason,
		&banExpires,
	)

	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}

	node.Identity.PublicKey = publicKey
	node.Type = domain.NodeType(nodeType)

	return &node, nil
}

// IsNodeBanned verifica se um nó está banido
func (r *NodeRepository) IsNodeBanned(ctx context.Context, nodeID string) (bool, string, error) {
	query := `
		SELECT banned, ban_reason, ban_expires_at
		FROM nodes
		WHERE node_id = $1
	`

	var (
		banned    bool
		reason    *string
		expiresAt *time.Time
	)

	err := r.pool.QueryRow(ctx, query, nodeID).Scan(&banned, &reason, &expiresAt)
	if err != nil {
		if err == pgx.ErrNoRows {
			return false, "", nil
		}
		return false, "", err
	}

	// Verifica se o ban expirou
	if banned && expiresAt != nil && time.Now().After(*expiresAt) {
		// Ban expirou, remove
		r.UnbanNode(ctx, nodeID)
		return false, "", nil
	}

	reasonStr := ""
	if reason != nil {
		reasonStr = *reason
	}

	return banned, reasonStr, nil
}

// BanNode marca um nó como banido
func (r *NodeRepository) BanNode(ctx context.Context, ban *domain.NodeBan) error {
	query := `
		UPDATE nodes
		SET banned = TRUE, ban_reason = $2, ban_expires_at = $3
		WHERE node_id = $1
	`

	_, err := r.pool.Exec(ctx, query, ban.BannedNodeID, ban.Reason, ban.ExpiresAt)
	return err
}

// UnbanNode remove o ban de um nó
func (r *NodeRepository) UnbanNode(ctx context.Context, nodeID string) error {
	query := `
		UPDATE nodes
		SET banned = FALSE, ban_reason = NULL, ban_expires_at = NULL
		WHERE node_id = $1
	`

	_, err := r.pool.Exec(ctx, query, nodeID)
	return err
}

// SaveModerationAction salva uma ação de moderação
func (r *NodeRepository) SaveModerationAction(ctx context.Context, action *domain.ModerationAction) error {
	query := `
		INSERT INTO moderation_actions (action_id, event_type, target_id, reason, duration_seconds, moderator_id, signature, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
	`

	_, err := r.pool.Exec(ctx, query,
		action.ActionID,
		int(action.Type),
		action.TargetID,
		action.Reason,
		int64(action.Duration.Seconds()),
		action.Moderator.ModeratorID,
		action.Signature,
		action.CreatedAt,
	)

	return err
}

// SaveContentBan salva um banimento de conteúdo
func (r *NodeRepository) SaveContentBan(ctx context.Context, ban *domain.ContentBan) error {
	query := `
		INSERT INTO content_bans (content_hash, reason, category, banned_at, moderator_id)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (content_hash) DO UPDATE SET
			reason = EXCLUDED.reason,
			category = EXCLUDED.category
	`

	_, err := r.pool.Exec(ctx, query,
		ban.ContentHash,
		ban.Reason,
		ban.Category,
		ban.BannedAt,
		ban.ModeratorID,
	)

	return err
}

// IsContentBanned verifica se um conteúdo está banido
func (r *NodeRepository) IsContentBanned(ctx context.Context, contentHash string) (bool, error) {
	query := `SELECT EXISTS(SELECT 1 FROM content_bans WHERE content_hash = $1)`

	var exists bool
	err := r.pool.QueryRow(ctx, query, contentHash).Scan(&exists)
	return exists, err
}

// GetModerator obtém um moderador pelo ID
func (r *NodeRepository) GetModerator(ctx context.Context, moderatorID string) (*domain.GlobalModerator, error) {
	query := `
		SELECT moderator_id, public_key, name, created_at, active
		FROM global_moderators
		WHERE moderator_id = $1
	`

	var mod domain.GlobalModerator
	err := r.pool.QueryRow(ctx, query, moderatorID).Scan(
		&mod.ModeratorID,
		&mod.PublicKey,
		&mod.Name,
		&mod.CreatedAt,
		&mod.Active,
	)

	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}

	return &mod, nil
}

// AddModerator adiciona um novo moderador global
func (r *NodeRepository) AddModerator(ctx context.Context, mod *domain.GlobalModerator) error {
	query := `
		INSERT INTO global_moderators (moderator_id, public_key, name, created_at, active)
		VALUES ($1, $2, $3, $4, $5)
	`

	_, err := r.pool.Exec(ctx, query,
		mod.ModeratorID,
		mod.PublicKey,
		mod.Name,
		mod.CreatedAt,
		mod.Active,
	)

	return err
}

// Ping verifica a conectividade com o PostgreSQL
func (r *NodeRepository) Ping(ctx context.Context) error {
	return r.pool.Ping(ctx)
}

// LogGovernanceAction registra uma ação de governança auditada
func (r *NodeRepository) LogGovernanceAction(ctx context.Context, requesterID, action, targetID, channelID string, signature []byte) error {
	query := `
		INSERT INTO governance_audit_log (timestamp, requester_id, action, target_id, channel_id, signature)
		VALUES ($1, $2, $3, $4, $5, $6)
	`
	_, err := r.pool.Exec(ctx, query, time.Now().UnixNano(), requesterID, action, targetID, channelID, signature)
	return err
}

// -- Cluster Peers (High Availability) --

// ClusterPeer representa um peer do cluster
type ClusterPeer struct {
	PeerID    string
	Address   string
	PublicKey []byte
	Status    string // 'pending', 'active'
	LastSeen  time.Time
}

// UpsertClusterPeer insere ou atualiza um peer do cluster
func (r *NodeRepository) UpsertClusterPeer(ctx context.Context, peer *ClusterPeer) error {
	query := `
		INSERT INTO cluster_peers (peer_id, address, public_key, status, last_seen)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (peer_id) DO UPDATE SET
			address = EXCLUDED.address,
			public_key = EXCLUDED.public_key,
			status = EXCLUDED.status,
			last_seen = EXCLUDED.last_seen
	`
	_, err := r.pool.Exec(ctx, query, peer.PeerID, peer.Address, peer.PublicKey, peer.Status, peer.LastSeen)
	return err
}

// GetClusterPeer obtém um peer pelo ID
func (r *NodeRepository) GetClusterPeer(ctx context.Context, peerID string) (*ClusterPeer, error) {
	query := `SELECT peer_id, address, public_key, status, last_seen FROM cluster_peers WHERE peer_id = $1`
	var peer ClusterPeer
	err := r.pool.QueryRow(ctx, query, peerID).Scan(&peer.PeerID, &peer.Address, &peer.PublicKey, &peer.Status, &peer.LastSeen)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return &peer, nil
}

// GetActiveClusterPeers retorna todos os peers ativos
func (r *NodeRepository) GetActiveClusterPeers(ctx context.Context) ([]*ClusterPeer, error) {
	query := `SELECT peer_id, address, public_key, status, last_seen FROM cluster_peers WHERE status = 'active'`
	rows, err := r.pool.Query(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var peers []*ClusterPeer
	for rows.Next() {
		var p ClusterPeer
		if err := rows.Scan(&p.PeerID, &p.Address, &p.PublicKey, &p.Status, &p.LastSeen); err != nil {
			return nil, err
		}
		peers = append(peers, &p)
	}
	return peers, nil
}

// UpdateClusterPeerStatus atualiza o status de um peer
func (r *NodeRepository) UpdateClusterPeerStatus(ctx context.Context, peerID, status string) error {
	query := `UPDATE cluster_peers SET status = $2, last_seen = NOW() WHERE peer_id = $1`
	_, err := r.pool.Exec(ctx, query, peerID, status)
	return err
}

// GetAllNodes retorna todos os nós registrados (para initial sync)
func (r *NodeRepository) GetAllNodes(ctx context.Context) ([]*domain.NodeIdentity, error) {
	query := `SELECT node_id, public_key FROM nodes`
	rows, err := r.pool.Query(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var nodes []*domain.NodeIdentity
	for rows.Next() {
		var n domain.NodeIdentity
		if err := rows.Scan(&n.NodeID, &n.PublicKey); err != nil {
			return nil, err
		}
		nodes = append(nodes, &n)
	}
	return nodes, nil
}
