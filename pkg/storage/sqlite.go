package storage

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite" // Driver Pure Go
)

type Storage struct {
	db     *sql.DB
	dbPath string
}

type Message struct {
	ID        int64
	MessageID string
	SenderID  string
	Target    string // #channel or NodeID
	Content   string // Decrypted content
	Timestamp time.Time
}

type ChatItem struct {
	ID          string
	Name        string // #channel or @User
	Type        string // "CHANNEL" or "DM"
	Avatar      string // Base64
	LastMessage time.Time
	UnreadCount int
}

type Member struct {
	NodeID    string
	ChannelID string
	Role      string
	LastSeen  time.Time
}

type WikiPage struct {
	ID        string
	ChannelID string
	Title     string
	Content   string
	UpdatedAt time.Time
}

type ChannelTag struct {
	ID        string
	ChannelID string
	Name      string
	Color     string
}

type MessageTag struct {
	MessageID string
	TagID     string
	TaggedBy  string
	TagName   string // Enriched
	TagColor  string // Enriched
}

type ChannelConfig struct {
	ChannelID string
	Key       string
	Value     string
}

// NewStorage inicializa o banco de dados SQLite local
func NewStorage(dbFilename string) (*Storage, error) {
	// Definir diretório de dados
	configDir, err := os.UserConfigDir()
	if err != nil {
		configDir = "."
	}
	appDir := filepath.Join(configDir, "Enxame")
	if err := os.MkdirAll(appDir, 0755); err != nil {
		return nil, err
	}

	if dbFilename == "" {
		dbFilename = "messages.db"
	}
	dbPath := filepath.Join(appDir, dbFilename)

	// Abrir conexão
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, err
	}

	s := &Storage{db: db, dbPath: dbPath}
	if err := s.initSchema(); err != nil {
		db.Close()
		return nil, err
	}

	return s, nil
}

func (s *Storage) DBPath() string {
	return s.dbPath
}

// initSchema cria as tabelas necessárias
func (s *Storage) initSchema() error {
	queries := []string{
		// Tabela de mensagens com message_id único para deduplicação
		`CREATE TABLE IF NOT EXISTS messages (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			message_id TEXT UNIQUE, 
			sender_id TEXT,
			target TEXT,
			content TEXT,
			timestamp DATETIME
		)`,
		`CREATE TABLE IF NOT EXISTS channels (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT UNIQUE,
			key_hex TEXT,
			avatar TEXT
		)`,
		`CREATE TABLE IF NOT EXISTS channel_members (
			channel_id TEXT,
			node_id TEXT,
			role TEXT,
			last_seen DATETIME,
			PRIMARY KEY (channel_id, node_id)
		)`,
		`CREATE TABLE IF NOT EXISTS dm_sessions (
			dm_id TEXT PRIMARY KEY,
			peer_id TEXT
		)`,
		`CREATE TABLE IF NOT EXISTS wiki_pages (
			id TEXT PRIMARY KEY, 
			channel_id TEXT,
			title TEXT,
			content TEXT,
			updated_at DATETIME
		)`,
		`CREATE TABLE IF NOT EXISTS channel_config (
			channel_id TEXT,
			key TEXT,
			value TEXT,
			PRIMARY KEY (channel_id, key)
		)`,
		// Configurações persistentes do sistema (ex: known_cores)
		`CREATE TABLE IF NOT EXISTS system_config (
			key TEXT PRIMARY KEY,
			value TEXT
		)`,
	}

	// Migration: Add avatar column if it doesn't exist (simplificação para MVP)
	s.db.Exec("ALTER TABLE channels ADD COLUMN avatar TEXT")

	for _, q := range queries {
		if _, err := s.db.Exec(q); err != nil {
			return fmt.Errorf("failed to init schema: %w", err)
		}
	}

	// Tag Tables
	tagQueries := []string{
		`CREATE TABLE IF NOT EXISTS channel_tags (
			id TEXT PRIMARY KEY, 
			channel_id TEXT, 
			name TEXT, 
			color TEXT
		)`,
		`CREATE TABLE IF NOT EXISTS message_tags (
			message_id TEXT, 
			tag_id TEXT, 
			tagged_by TEXT, 
			PRIMARY KEY (message_id, tag_id)
		)`,
	}
	for _, q := range tagQueries {
		if _, err := s.db.Exec(q); err != nil {
			return fmt.Errorf("failed to init tag schema: %w", err)
		}
	}
	return nil
}

// StoreMessage salva uma mensagem com deduplicação
func (s *Storage) StoreMessage(msgID, senderID, target, content string) error {
	_, err := s.db.Exec(
		"INSERT OR IGNORE INTO messages (message_id, sender_id, target, content, timestamp) VALUES (?, ?, ?, ?, ?)",
		msgID, senderID, target, content, time.Now(),
	)
	return err
}

// GetMessages retorna histórico recente de um chat (canal ou p2p)
func (s *Storage) GetMessages(target string, limit int) ([]Message, error) {
	rows, err := s.db.Query(
		"SELECT id, sender_id, target, content, timestamp FROM messages WHERE target = ? ORDER BY id DESC LIMIT ?",
		target, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var msgs []Message
	for rows.Next() {
		var m Message
		if err := rows.Scan(&m.ID, &m.SenderID, &m.Target, &m.Content, &m.Timestamp); err != nil {
			continue
		}
		msgs = append(msgs, m) // Ordem inversa (mais recente primeiro)
	}

	// Inverter para ordem cronológica
	for i, j := 0, len(msgs)-1; i < j; i, j = i+1, j-1 {
		msgs[i], msgs[j] = msgs[j], msgs[i]
	}

	return msgs, nil
}

// SaveChannelKey salva a chave de um canal
func (s *Storage) SaveChannelKey(name, keyHex string) error {
	_, err := s.db.Exec("INSERT OR REPLACE INTO channels (name, key_hex, avatar) VALUES (?, ?, (SELECT avatar FROM channels WHERE name = ?))", name, keyHex, name)
	return err
}

func (s *Storage) UpdateChannelAvatar(name, avatar string) error {
	_, err := s.db.Exec("UPDATE channels SET avatar = ? WHERE name = ?", avatar, name)
	return err
}

// GetChannelKey recupera a chave de um canal
func (s *Storage) GetChannelKey(name string) (string, error) {
	var keyHex string
	err := s.db.QueryRow("SELECT key_hex FROM channels WHERE name = ?", name).Scan(&keyHex)
	if err != nil {
		return "", err
	}
	return keyHex, nil
}

// DMSessions Logic
func (s *Storage) UpsertDMSession(dmID, peerID string) error {
	_, err := s.db.Exec("INSERT OR REPLACE INTO dm_sessions (dm_id, peer_id) VALUES (?, ?)", dmID, peerID)
	return err
}

func (s *Storage) GetDMPeer(dmID string) (string, error) {
	var peerID string
	err := s.db.QueryRow("SELECT peer_id FROM dm_sessions WHERE dm_id = ?", dmID).Scan(&peerID)
	return peerID, err
}

// GetRecentChats returns channels and DMs sorted by last message
func (s *Storage) GetRecentChats() ([]ChatItem, error) {
	// 1. Get all distinct targets from messages with their max timestamp
	rows, err := s.db.Query(`
		SELECT target, MAX(timestamp) as last_msg 
		FROM messages 
		GROUP BY target 
		ORDER BY last_msg DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	// Load channel avatars
	avatars := make(map[string]string)
	chRows, _ := s.db.Query("SELECT name, avatar FROM channels")
	if chRows != nil {
		defer chRows.Close()
		for chRows.Next() {
			var n, a sql.NullString
			chRows.Scan(&n, &a)
			if n.Valid {
				avatars[n.String] = a.String
			}
		}
	}

	var chats []ChatItem
	seen := make(map[string]bool)

	for rows.Next() {
		var target string
		var ts time.Time
		if err := rows.Scan(&target, &ts); err != nil {
			continue
		}

		// Determine Type and Name
		cType := "CHANNEL"
		name := target
		if len(target) > 0 && target[0] != '#' {
			cType = "DM"
			// Try to resolve name from dm_sessions
			if peer, err := s.GetDMPeer(target); err == nil && peer != "" {
				name = "@" + peer // Display as @NodeID
			}
		}

		chats = append(chats, ChatItem{
			ID:          target,
			Name:        name,
			Type:        cType,
			Avatar:      avatars[target],
			LastMessage: ts,
		})
		seen[target] = true
	}

	// 2. Add channels that might not have messages yet (optional, but good for UI)
	allChannels, _ := s.GetAllChannels()
	for _, ch := range allChannels {
		if !seen[ch] {
			chats = append(chats, ChatItem{
				ID:          ch,
				Name:        ch,
				Type:        "CHANNEL",
				Avatar:      avatars[ch],
				LastMessage: time.Time{}, // Oldest
			})
		}
	}

	// Re-sort if we added empty channels (they go to bottom)
	// For MVP, relying on initial query + append is 'ok', but accurate sort is better.
	// We'll trust the user accepts joined channels without messages appearing at bottom.

	return chats, nil
}

// GetAllChannels retorna lista de canais salvos
func (s *Storage) GetAllChannels() ([]string, error) {
	rows, err := s.db.Query("SELECT name FROM channels ORDER BY name")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var channels []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err == nil {
			channels = append(channels, name)
		}
	}
	return channels, nil
}

// UpsertMember updates or inserts a member record
func (s *Storage) UpsertMember(channelID, nodeID, role string) error {
	_, err := s.db.Exec(`
		INSERT INTO channel_members (channel_id, node_id, role, last_seen)
		VALUES (?, ?, ?, ?)
		ON CONFLICT(channel_id, node_id) DO UPDATE SET
			last_seen = excluded.last_seen,
			role = CASE WHEN excluded.role != '' THEN excluded.role ELSE role END
	`, channelID, nodeID, role, time.Now())
	return err
}

// GetChannelMembers retrieves all members for a channel
func (s *Storage) GetChannelMembers(channelID string) ([]Member, error) {
	rows, err := s.db.Query("SELECT node_id, role, last_seen FROM channel_members WHERE channel_id = ? ORDER BY role DESC, last_seen DESC", channelID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var members []Member
	for rows.Next() {
		var m Member
		m.ChannelID = channelID
		if err := rows.Scan(&m.NodeID, &m.Role, &m.LastSeen); err != nil {
			continue
		}
		members = append(members, m)
	}
	return members, nil
}

func (s *Storage) GetMember(channelID, nodeID string) (Member, error) {
	var m Member
	err := s.db.QueryRow("SELECT node_id, role, last_seen FROM channel_members WHERE channel_id = ? AND node_id = ?", channelID, nodeID).Scan(&m.NodeID, &m.Role, &m.LastSeen)
	if err != nil {
		return Member{}, err
	}
	m.ChannelID = channelID
	return m, nil
}

// Wiki Methods
func (s *Storage) SaveWikiPage(p WikiPage) error {
	_, err := s.db.Exec(`
		INSERT INTO wiki_pages (id, channel_id, title, content, updated_at) 
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET 
			title=excluded.title, 
			content=excluded.content, 
			updated_at=excluded.updated_at
	`, p.ID, p.ChannelID, p.Title, p.Content, p.UpdatedAt)
	return err
}

func (s *Storage) GetWikiPages(channelID string) ([]WikiPage, error) {
	rows, err := s.db.Query("SELECT id, title, content, updated_at FROM wiki_pages WHERE channel_id = ? ORDER BY title", channelID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var pages []WikiPage
	for rows.Next() {
		var p WikiPage
		p.ChannelID = channelID
		if err := rows.Scan(&p.ID, &p.Title, &p.Content, &p.UpdatedAt); err != nil {
			continue
		}
		pages = append(pages, p)
	}
	return pages, nil
}

// Channel Config Methods
func (s *Storage) SetChannelConfig(channelID, key, value string) error {
	_, err := s.db.Exec(`
		INSERT INTO channel_config (channel_id, key, value) 
		VALUES (?, ?, ?)
		ON CONFLICT(channel_id, key) DO UPDATE SET value=excluded.value
	`, channelID, key, value)
	return err
}

func (s *Storage) GetChannelConfig(channelID, key string) (string, error) {
	var value string
	err := s.db.QueryRow("SELECT value FROM channel_config WHERE channel_id = ? AND key = ?", channelID, key).Scan(&value)
	if err == sql.ErrNoRows {
		return "", nil // Not found is empty string
	}
	return value, err
}

// Tag Methods
func (s *Storage) CreateTag(t ChannelTag) error {
	_, err := s.db.Exec(`INSERT OR REPLACE INTO channel_tags (id, channel_id, name, color) VALUES (?, ?, ?, ?)`, t.ID, t.ChannelID, t.Name, t.Color)
	return err
}

func (s *Storage) GetTags(channelID string) ([]ChannelTag, error) {
	rows, err := s.db.Query("SELECT id, channel_id, name, color FROM channel_tags WHERE channel_id = ?", channelID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var tags []ChannelTag
	for rows.Next() {
		var t ChannelTag
		if err := rows.Scan(&t.ID, &t.ChannelID, &t.Name, &t.Color); err != nil {
			continue
		}
		tags = append(tags, t)
	}
	return tags, nil
}

func (s *Storage) AddMessageTag(mt MessageTag) error {
	_, err := s.db.Exec(`INSERT OR IGNORE INTO message_tags (message_id, tag_id, tagged_by) VALUES (?, ?, ?)`, mt.MessageID, mt.TagID, mt.TaggedBy)
	return err
}

func (s *Storage) GetMessageTags(channelID string) (map[string][]MessageTag, error) {
	// Join with channel_tags to get name/color
	rows, err := s.db.Query(`
		SELECT mt.message_id, mt.tag_id, mt.tagged_by, ct.name, ct.color 
		FROM message_tags mt 
		JOIN channel_tags ct ON mt.tag_id = ct.id 
		WHERE ct.channel_id = ?`, channelID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make(map[string][]MessageTag)
	for rows.Next() {
		var mt MessageTag
		if err := rows.Scan(&mt.MessageID, &mt.TagID, &mt.TaggedBy, &mt.TagName, &mt.TagColor); err != nil {
			continue
		}
		result[mt.MessageID] = append(result[mt.MessageID], mt)
	}
	return result, nil
}

// -- System Config (KnownCores, etc) --

// GetConfig obtém um valor de configuração do sistema
func (s *Storage) GetConfig(key string) (string, error) {
	var value string
	err := s.db.QueryRow("SELECT value FROM system_config WHERE key = ?", key).Scan(&value)
	if err != nil {
		if err == sql.ErrNoRows {
			return "", nil
		}
		return "", err
	}
	return value, nil
}

// SetConfig define um valor de configuração do sistema
func (s *Storage) SetConfig(key, value string) error {
	_, err := s.db.Exec(`
		INSERT INTO system_config (key, value) VALUES (?, ?)
		ON CONFLICT(key) DO UPDATE SET value = excluded.value
	`, key, value)
	return err
}

func (s *Storage) Close() {
	if s.db != nil {
		s.db.Close()
	}
}
