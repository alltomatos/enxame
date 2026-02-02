package redis

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/go-redis/redis/v8"
	"github.com/goautomatik/core-server/internal/domain"
)

const (
	// Prefixos de chaves Redis
	keyPrefixNode       = "node:"
	keyPrefixRelay      = "relay:"
	keyActiveRelays     = "active_relays"
	keyModerationEvents = "moderation_events"
)

// NodeManager gerencia o estado dos nós em tempo real no Redis
type NodeManager struct {
	client       *redis.Client
	heartbeatTTL time.Duration
}

// NewNodeManager cria um novo gerenciador de nós
func NewNodeManager(client *redis.Client, heartbeatTTL time.Duration) *NodeManager {
	return &NodeManager{
		client:       client,
		heartbeatTTL: heartbeatTTL,
	}
}

// nodeData representa os dados do nó armazenados no Redis
type nodeData struct {
	NodeID       string   `json:"node_id"`
	PublicKey    []byte   `json:"public_key"`
	Type         int      `json:"type"`
	Status       int      `json:"status"`
	Endpoints    []string `json:"endpoints"`
	Version      string   `json:"version"`
	Capabilities []string `json:"capabilities"`
	Region       string   `json:"region"`
	RegisteredAt int64    `json:"registered_at"`
	LastSeen     int64    `json:"last_seen"`
}

// RegisterNode registra um nó no Redis com TTL
func (m *NodeManager) RegisterNode(ctx context.Context, node *domain.Node) error {
	data := nodeData{
		NodeID:       node.Identity.NodeID,
		PublicKey:    node.Identity.PublicKey,
		Type:         int(node.Type),
		Status:       int(node.Status),
		Endpoints:    node.Endpoints,
		Version:      node.Version,
		Capabilities: node.Capabilities,
		Region:       node.Region,
		RegisteredAt: node.RegisteredAt.Unix(),
		LastSeen:     time.Now().Unix(),
	}

	jsonData, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("failed to marshal node data: %w", err)
	}

	key := keyPrefixNode + node.Identity.NodeID
	if err := m.client.Set(ctx, key, jsonData, m.heartbeatTTL).Err(); err != nil {
		return fmt.Errorf("failed to set node in redis: %w", err)
	}

	// Se for um relay, adiciona ao conjunto de relays ativos
	if node.Type == domain.NodeTypeRelay {
		if err := m.addToActiveRelays(ctx, node.Identity.NodeID, node.Region); err != nil {
			return err
		}
	}

	return nil
}

// RefreshHeartbeat renova o TTL de um nó
func (m *NodeManager) RefreshHeartbeat(ctx context.Context, nodeID string, status domain.NodeStatus, metrics *domain.NodeMetrics) error {
	key := keyPrefixNode + nodeID

	// Obtém os dados atuais
	data, err := m.client.Get(ctx, key).Bytes()
	if err != nil {
		if err == redis.Nil {
			return fmt.Errorf("node not found: %s", nodeID)
		}
		return fmt.Errorf("failed to get node from redis: %w", err)
	}

	var node nodeData
	if err := json.Unmarshal(data, &node); err != nil {
		return fmt.Errorf("failed to unmarshal node data: %w", err)
	}

	// Atualiza status e last_seen
	node.Status = int(status)
	node.LastSeen = time.Now().Unix()

	jsonData, err := json.Marshal(node)
	if err != nil {
		return fmt.Errorf("failed to marshal node data: %w", err)
	}

	// Renova o TTL
	if err := m.client.Set(ctx, key, jsonData, m.heartbeatTTL).Err(); err != nil {
		return fmt.Errorf("failed to refresh heartbeat in redis: %w", err)
	}

	return nil
}

// GetNode obtém um nó pelo ID
func (m *NodeManager) GetNode(ctx context.Context, nodeID string) (*domain.Node, error) {
	key := keyPrefixNode + nodeID

	data, err := m.client.Get(ctx, key).Bytes()
	if err != nil {
		if err == redis.Nil {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get node from redis: %w", err)
	}

	var nd nodeData
	if err := json.Unmarshal(data, &nd); err != nil {
		return nil, fmt.Errorf("failed to unmarshal node data: %w", err)
	}

	return &domain.Node{
		Identity: domain.NodeIdentity{
			NodeID:    nd.NodeID,
			PublicKey: nd.PublicKey,
		},
		Type:         domain.NodeType(nd.Type),
		Status:       domain.NodeStatus(nd.Status),
		Endpoints:    nd.Endpoints,
		Version:      nd.Version,
		Capabilities: nd.Capabilities,
		Region:       nd.Region,
		RegisteredAt: time.Unix(nd.RegisteredAt, 0),
		LastSeen:     time.Unix(nd.LastSeen, 0),
	}, nil
}

// GetActiveRelays retorna os relays ativos, opcionalmente filtrados por região
func (m *NodeManager) GetActiveRelays(ctx context.Context, preferredRegion string, limit int, excludeIDs []string) ([]*domain.Node, error) {
	// Cria um set de IDs a excluir para busca rápida
	excludeSet := make(map[string]bool)
	for _, id := range excludeIDs {
		excludeSet[id] = true
	}

	// Obtém todos os IDs de relays ativos
	relayKeys, err := m.client.Keys(ctx, keyPrefixNode+"*").Result()
	if err != nil {
		return nil, fmt.Errorf("failed to get relay keys: %w", err)
	}

	var relays []*domain.Node
	preferredRelays := make([]*domain.Node, 0)
	otherRelays := make([]*domain.Node, 0)

	for _, key := range relayKeys {
		nodeID := key[len(keyPrefixNode):]
		if excludeSet[nodeID] {
			continue
		}

		node, err := m.GetNode(ctx, nodeID)
		if err != nil || node == nil {
			continue
		}

		if node.Type != domain.NodeTypeRelay {
			continue
		}

		// Separa por região preferida
		if preferredRegion != "" && node.Region == preferredRegion {
			preferredRelays = append(preferredRelays, node)
		} else {
			otherRelays = append(otherRelays, node)
		}
	}

	// Prioriza relays da região preferida
	relays = append(relays, preferredRelays...)
	relays = append(relays, otherRelays...)

	// Aplica limite
	if limit > 0 && len(relays) > limit {
		relays = relays[:limit]
	}

	return relays, nil
}

// RemoveNode remove um nó do Redis
func (m *NodeManager) RemoveNode(ctx context.Context, nodeID string) error {
	key := keyPrefixNode + nodeID

	// Verifica se é um relay antes de remover
	node, _ := m.GetNode(ctx, nodeID)
	if node != nil && node.Type == domain.NodeTypeRelay {
		m.removeFromActiveRelays(ctx, nodeID)
	}

	return m.client.Del(ctx, key).Err()
}

// addToActiveRelays adiciona um relay ao conjunto de relays ativos
func (m *NodeManager) addToActiveRelays(ctx context.Context, nodeID string, region string) error {
	member := redis.Z{
		Score:  float64(time.Now().Unix()),
		Member: nodeID,
	}
	return m.client.ZAdd(ctx, keyActiveRelays, &member).Err()
}

// removeFromActiveRelays remove um relay do conjunto de relays ativos
func (m *NodeManager) removeFromActiveRelays(ctx context.Context, nodeID string) error {
	return m.client.ZRem(ctx, keyActiveRelays, nodeID).Err()
}

// PublishModerationEvent publica um evento de moderação via Pub/Sub
func (m *NodeManager) PublishModerationEvent(ctx context.Context, event *domain.ModerationEvent) error {
	eventData, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("failed to marshal moderation event: %w", err)
	}

	return m.client.Publish(ctx, keyModerationEvents, eventData).Err()
}

// SubscribeToModerationEvents retorna um canal para receber eventos de moderação
func (m *NodeManager) SubscribeToModerationEvents(ctx context.Context) (<-chan *domain.ModerationEvent, error) {
	pubsub := m.client.Subscribe(ctx, keyModerationEvents)

	eventChan := make(chan *domain.ModerationEvent, 100)

	go func() {
		defer close(eventChan)
		defer pubsub.Close()

		ch := pubsub.Channel()
		for {
			select {
			case <-ctx.Done():
				return
			case msg, ok := <-ch:
				if !ok {
					return
				}

				var event domain.ModerationEvent
				if err := json.Unmarshal([]byte(msg.Payload), &event); err != nil {
					continue
				}

				select {
				case eventChan <- &event:
				default:
					// Canal cheio, descarta evento antigo
				}
			}
		}
	}()

	return eventChan, nil
}

// GetNodeCount retorna a contagem de nós ativos por tipo
func (m *NodeManager) GetNodeCount(ctx context.Context) (map[domain.NodeType]int, error) {
	counts := make(map[domain.NodeType]int)

	keys, err := m.client.Keys(ctx, keyPrefixNode+"*").Result()
	if err != nil {
		return nil, err
	}

	for _, key := range keys {
		nodeID := key[len(keyPrefixNode):]
		node, err := m.GetNode(ctx, nodeID)
		if err != nil || node == nil {
			continue
		}
		counts[node.Type]++
	}

	return counts, nil
}

// Ping verifica a conectividade com o Redis
func (m *NodeManager) Ping(ctx context.Context) error {
	return m.client.Ping(ctx).Err()
}
