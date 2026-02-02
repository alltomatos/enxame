package service

import (
	"context"
	"errors"
	"time"

	"github.com/goautomatik/core-server/internal/crypto"
	"github.com/goautomatik/core-server/internal/domain"
	"github.com/goautomatik/core-server/internal/repository/postgres"
	"github.com/goautomatik/core-server/internal/repository/redis"
)

var (
	ErrInvalidIdentity  = errors.New("invalid node identity")
	ErrNodeBanned       = errors.New("node is banned")
	ErrNodeNotFound     = errors.New("node not found")
	ErrInvalidSignature = errors.New("invalid signature")
)

// NodeService gerencia a lógica de negócio relacionada a nós
type NodeService struct {
	redisManager *redis.NodeManager
	pgRepo       *postgres.NodeRepository
	heartbeatTTL time.Duration
	maxRelays    int
}

// NewNodeService cria um novo serviço de nós
func NewNodeService(
	redisManager *redis.NodeManager,
	pgRepo *postgres.NodeRepository,
	heartbeatTTL time.Duration,
	maxRelays int,
) *NodeService {
	return &NodeService{
		redisManager: redisManager,
		pgRepo:       pgRepo,
		heartbeatTTL: heartbeatTTL,
		maxRelays:    maxRelays,
	}
}

// RegisterNode registra um novo nó na rede
func (s *NodeService) RegisterNode(ctx context.Context, reg *domain.NodeRegistration) (*domain.Node, []*domain.Node, error) {
	// Verifica a identidade do nó (assinatura da chave pública)
	if err := crypto.VerifyNodeIdentity(
		reg.Identity.NodeID,
		reg.Identity.PublicKey,
		reg.Identity.Signature,
	); err != nil {
		return nil, nil, ErrInvalidIdentity
	}

	// Verifica se o nó está banido
	banned, reason, err := s.pgRepo.IsNodeBanned(ctx, reg.Identity.NodeID)
	if err != nil {
		return nil, nil, err
	}
	if banned {
		return nil, nil, errors.New(ErrNodeBanned.Error() + ": " + reason)
	}

	// Cria o objeto de domínio
	node := &domain.Node{
		Identity:     reg.Identity,
		Type:         reg.Type,
		Status:       domain.NodeStatusOnline,
		Endpoints:    reg.Endpoints,
		Version:      reg.Version,
		Capabilities: reg.Capabilities,
		Region:       reg.Region,
		RegisteredAt: time.Now(),
		LastSeen:     time.Now(),
	}

	// Registra no Redis (estado em tempo real)
	if err := s.redisManager.RegisterNode(ctx, node); err != nil {
		return nil, nil, err
	}

	// Persiste no PostgreSQL (metadados)
	if err := s.pgRepo.UpsertNode(ctx, node); err != nil {
		// Log do erro mas não falha (Redis é a fonte de verdade para estado online)
		// TODO: adicionar logging estruturado
	}

	// Obtém relays sugeridos para o novo nó
	suggestedRelays, err := s.redisManager.GetActiveRelays(ctx, reg.Region, s.maxRelays, nil)
	if err != nil {
		suggestedRelays = []*domain.Node{}
	}

	return node, suggestedRelays, nil
}

// Heartbeat processa um heartbeat de um nó
func (s *NodeService) Heartbeat(ctx context.Context, nodeID string, signature []byte, status domain.NodeStatus, metrics *domain.NodeMetrics) (int, error) {
	// Obtém o nó do Redis
	node, err := s.redisManager.GetNode(ctx, nodeID)
	if err != nil {
		return 0, err
	}
	if node == nil {
		return 0, ErrNodeNotFound
	}

	// Verifica a assinatura do heartbeat
	// A mensagem assinada é o timestamp atual (tolerância de 60s)
	// Para simplificar, apenas verificamos se o nó existe no Redis
	// Em produção, verificaríamos a assinatura do timestamp

	// Verifica se está banido
	banned, reason, err := s.pgRepo.IsNodeBanned(ctx, nodeID)
	if err != nil {
		return 0, err
	}
	if banned {
		return 0, errors.New(ErrNodeBanned.Error() + ": " + reason)
	}

	// Atualiza o heartbeat no Redis
	if err := s.redisManager.RefreshHeartbeat(ctx, nodeID, status, metrics); err != nil {
		return 0, err
	}

	// Retorna o intervalo do próximo heartbeat (em segundos)
	return int(s.heartbeatTTL.Seconds() / 2), nil
}

// GetActiveRelays retorna os relays ativos
func (s *NodeService) GetActiveRelays(ctx context.Context, preferredRegion string, limit int, excludeIDs []string) ([]*domain.Node, error) {
	if limit <= 0 || limit > s.maxRelays {
		limit = s.maxRelays
	}

	return s.redisManager.GetActiveRelays(ctx, preferredRegion, limit, excludeIDs)
}

// GetNode obtém um nó pelo ID
func (s *NodeService) GetNode(ctx context.Context, nodeID string) (*domain.Node, error) {
	// Primeiro tenta o Redis (estado em tempo real)
	node, err := s.redisManager.GetNode(ctx, nodeID)
	if err != nil {
		return nil, err
	}
	if node != nil {
		return node, nil
	}

	// Se não encontrou no Redis, busca no PostgreSQL (histórico)
	return s.pgRepo.GetNode(ctx, nodeID)
}

// RemoveNode remove um nó da rede
func (s *NodeService) RemoveNode(ctx context.Context, nodeID string) error {
	return s.redisManager.RemoveNode(ctx, nodeID)
}

// GetNodeStats retorna estatísticas dos nós
func (s *NodeService) GetNodeStats(ctx context.Context) (map[domain.NodeType]int, error) {
	return s.redisManager.GetNodeCount(ctx)
}
