package relay

import (
	"log"
	"sync"
	"time"

	pbv1 "github.com/goautomatik/core-server/pkg/pb/v1"
)

// Session representa uma sessão ativa de um nó conectado
type Session struct {
	NodeID       string
	PublicKey    []byte
	SessionID    string
	ConnectedAt  time.Time
	LastActivity time.Time
	SendChan     chan *pbv1.Envelope // Canal para enviar mensagens ao nó
}

// SessionManager gerencia todas as sessões ativas no Relay
type SessionManager struct {
	// Mapa de NodeID -> Session
	sessions sync.Map

	// Blacklist global sincronizada do Core Server
	blacklist sync.Map

	// Canais (Pub/Sub) - ChannelID -> sync.Map (Set of NodeIDs)
	channels sync.Map

	// Métricas
	activeConnections int64
	messagesRelayed   int64
	mu                sync.RWMutex
}

// NewSessionManager cria um novo gerenciador de sessões
func NewSessionManager() *SessionManager {
	return &SessionManager{}
}

// RegisterSession registra uma nova sessão
func (sm *SessionManager) RegisterSession(nodeID string, publicKey []byte) (*Session, error) {
	// Verifica blacklist
	if sm.IsBlacklisted(nodeID) {
		return nil, ErrNodeBlacklisted
	}

	session := &Session{
		NodeID:       nodeID,
		PublicKey:    publicKey,
		SessionID:    generateSessionID(),
		ConnectedAt:  time.Now(),
		LastActivity: time.Now(),
		SendChan:     make(chan *pbv1.Envelope, 100), // Buffer de 100 mensagens
	}

	// Fecha sessão anterior se existir
	if old, loaded := sm.sessions.LoadAndDelete(nodeID); loaded {
		oldSession := old.(*Session)
		close(oldSession.SendChan)
		log.Printf("[SessionManager] Replaced session for node %s", nodeID[:8])
	}

	sm.sessions.Store(nodeID, session)

	sm.mu.Lock()
	sm.activeConnections++
	sm.mu.Unlock()

	log.Printf("[SessionManager] Registered session for node %s (session: %s)", nodeID[:8], session.SessionID[:8])

	return session, nil
}

// UnregisterSession remove uma sessão
func (sm *SessionManager) UnregisterSession(nodeID string) {
	if val, loaded := sm.sessions.LoadAndDelete(nodeID); loaded {
		session := val.(*Session)
		close(session.SendChan)

		sm.mu.Lock()
		sm.activeConnections--
		sm.mu.Unlock()

		log.Printf("[SessionManager] Unregistered session for node %s", nodeID[:8])
	}
}

// GetSession obtém uma sessão pelo NodeID
func (sm *SessionManager) GetSession(nodeID string) (*Session, bool) {
	if val, ok := sm.sessions.Load(nodeID); ok {
		session := val.(*Session)
		session.LastActivity = time.Now()
		return session, true
	}
	return nil, false
}
func (sm *SessionManager) SubscribeToChannel(nodeID, channelID string) {
	val, _ := sm.channels.LoadOrStore(channelID, &sync.Map{})
	subscribers := val.(*sync.Map)
	subscribers.Store(nodeID, true)
	log.Printf("[SessionManager] Node %s subscribed to %s", nodeID[:8], channelID)
}

// UnsubscribeFromChannel remove um nó de um canal
func (sm *SessionManager) UnsubscribeFromChannel(nodeID, channelID string) {
	if val, ok := sm.channels.Load(channelID); ok {
		subscribers := val.(*sync.Map)
		subscribers.Delete(nodeID)
		log.Printf("[SessionManager] Node %s unsubscribed from %s", nodeID[:8], channelID)
	}
}

// ForwardEnvelope encaminha um envelope para o destinatário ou broadcast
func (sm *SessionManager) ForwardEnvelope(envelope *pbv1.Envelope) error {
	// Verifica se o remetente está na blacklist
	if sm.IsBlacklisted(envelope.SenderNodeId) {
		log.Printf("[SessionManager] ❌ Rejected message from blacklisted node %s", envelope.SenderNodeId[:8])
		return ErrNodeBlacklisted
	}

	// BROADCAST: Se TargetNodeId começa com '#', é um canal
	if len(envelope.TargetNodeId) > 0 && envelope.TargetNodeId[0] == '#' {
		return sm.broadcastEnvelope(envelope)
	}

	// 1-to-1
	// Busca a sessão do destinatário
	targetSession, exists := sm.GetSession(envelope.TargetNodeId)
	if !exists {
		log.Printf("[SessionManager] ⚠️ Target node %s not connected", envelope.TargetNodeId[:8])
		return ErrTargetNotConnected
	}

	return sm.sendToSession(targetSession, envelope)
}

// broadcastEnvelope envia mensagem para todos os inscritos no canal
func (sm *SessionManager) broadcastEnvelope(envelope *pbv1.Envelope) error {
	channelID := envelope.TargetNodeId
	val, ok := sm.channels.Load(channelID)
	if !ok {
		log.Printf("[SessionManager] ⚠️ Channel %s has no subscribers or does not exist", channelID)
		return nil // Não é erro, só ninguém ouvindo
	}

	subscribers := val.(*sync.Map)
	count := 0
	subscribers.Range(func(key, value interface{}) bool {
		targetNodeID := key.(string)

		// Não enviar de volta para o remetente (Echo suppression)
		if targetNodeID == envelope.SenderNodeId {
			return true
		}

		if session, exists := sm.GetSession(targetNodeID); exists {
			// Envia cópia (Best effort)
			// TODO: Clone envelope se for modificar (aqui não modificamos)
			select {
			case session.SendChan <- envelope:
				count++
			default:
				// Drop se buffer cheio
			}
		} else {
			// Cleanup lazy: Se não tem sessão, remove da inscrição?
			// Por enquanto mantemos, pois pode reconectar.
		}
		return true
	})

	sm.mu.Lock()
	sm.messagesRelayed += int64(count) // Conta cada entrega como relayed
	sm.mu.Unlock()

	log.Printf("[SessionManager] 📢 Broadcast to %s delivered to %d nodes", channelID, count)
	return nil
}

func (sm *SessionManager) sendToSession(session *Session, envelope *pbv1.Envelope) error {
	select {
	case session.SendChan <- envelope:
		sm.mu.Lock()
		sm.messagesRelayed++
		sm.mu.Unlock()

		log.Printf("[SessionManager] ✉️ Forwarded message from %s to %s (type: %v)",
			envelope.SenderNodeId[:8], session.NodeID[:8], envelope.MessageType)
		return nil

	default:
		log.Printf("[SessionManager] ⚠️ Buffer full for node %s, dropping message", session.NodeID[:8])
		return ErrBufferFull
	}
}

// AddToBlacklist adiciona um nó à blacklist
func (sm *SessionManager) AddToBlacklist(nodeID string) {
	sm.blacklist.Store(nodeID, time.Now())
	log.Printf("[SessionManager] 🚫 Added node %s to blacklist", nodeID[:8])

	// Desconecta o nó se estiver conectado
	sm.UnregisterSession(nodeID)

	// Remove de todos os canais? (Opcional, mas boa prática)
	// Implementação futura: iterar canais e remover.
}

// RemoveFromBlacklist remove um nó da blacklist
func (sm *SessionManager) RemoveFromBlacklist(nodeID string) {
	sm.blacklist.Delete(nodeID)
	log.Printf("[SessionManager] ✅ Removed node %s from blacklist", nodeID[:8])
}

// IsBlacklisted verifica se um nó está na blacklist
func (sm *SessionManager) IsBlacklisted(nodeID string) bool {
	_, exists := sm.blacklist.Load(nodeID)
	return exists
}

// GetConnectedNodeIDs retorna lista de nós conectados
func (sm *SessionManager) GetConnectedNodeIDs() []string {
	var nodeIDs []string
	sm.sessions.Range(func(key, value interface{}) bool {
		nodeIDs = append(nodeIDs, key.(string))
		return true
	})
	return nodeIDs
}

// GetStats retorna estatísticas do gerenciador
func (sm *SessionManager) GetStats() (activeConnections, messagesRelayed int64) {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	return sm.activeConnections, sm.messagesRelayed
}

// generateSessionID gera um ID de sessão único
func generateSessionID() string {
	return time.Now().Format("20060102150405") + randomHex(8)
}

func randomHex(n int) string {
	const letters = "0123456789abcdef"
	b := make([]byte, n)
	for i := range b {
		b[i] = letters[time.Now().UnixNano()%int64(len(letters))]
		time.Sleep(time.Nanosecond)
	}
	return string(b)
}
