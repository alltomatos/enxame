package service

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/goautomatik/core-server/internal/crypto"
	"github.com/goautomatik/core-server/internal/domain"
	"github.com/goautomatik/core-server/internal/repository/postgres"
	"github.com/goautomatik/core-server/internal/repository/redis"
	"github.com/google/uuid"
)

var (
	ErrUnauthorizedModerator = errors.New("unauthorized: not a global moderator")
	ErrInvalidModerationSign = errors.New("invalid moderation signature")
)

// ModerationService gerencia a lógica de moderação global
type ModerationService struct {
	redisManager *redis.NodeManager
	pgRepo       *postgres.NodeRepository
	verifier     *crypto.Verifier
}

// NewModerationService cria um novo serviço de moderação
func NewModerationService(
	redisManager *redis.NodeManager,
	pgRepo *postgres.NodeRepository,
	verifier *crypto.Verifier,
) *ModerationService {
	return &ModerationService{
		redisManager: redisManager,
		pgRepo:       pgRepo,
		verifier:     verifier,
	}
}

// BanNode processa um banimento de nó assinado por um moderador global
func (s *ModerationService) BanNode(ctx context.Context, action *domain.ModerationAction) error {
	// Verifica se é um moderador válido (chave mestre)
	if !s.verifier.IsMasterKey(action.Moderator.PublicKey) {
		return ErrUnauthorizedModerator
	}

	// Serializa a ação para verificação da assinatura
	actionData, err := s.serializeActionForSigning(action)
	if err != nil {
		return err
	}

	// Verifica a assinatura
	if err := s.verifier.VerifyMasterSignature(
		action.Moderator.PublicKey,
		actionData,
		action.Signature,
	); err != nil {
		return ErrInvalidModerationSign
	}

	// Cria o ban
	var expiresAt *time.Time
	if action.Duration > 0 {
		t := time.Now().Add(action.Duration)
		expiresAt = &t
	}

	ban := &domain.NodeBan{
		BannedNodeID:    action.TargetID,
		Reason:          action.Reason,
		DurationSeconds: int64(action.Duration.Seconds()),
		BannedAt:        time.Now(),
		ExpiresAt:       expiresAt,
		ModeratorID:     action.Moderator.ModeratorID,
	}

	// Persiste no PostgreSQL
	if err := s.pgRepo.BanNode(ctx, ban); err != nil {
		return err
	}

	// Salva a ação de moderação
	if err := s.pgRepo.SaveModerationAction(ctx, action); err != nil {
		// Log mas não falha
	}

	// Remove o nó do Redis (força desconexão)
	s.redisManager.RemoveNode(ctx, action.TargetID)

	// Propaga o evento via Pub/Sub
	event := &domain.ModerationEvent{
		EventID:   generateEventID(),
		Type:      domain.EventTypeNodeBanned,
		Timestamp: time.Now(),
		Payload:   ban,
		Signature: &domain.ModeratorSignature{
			ModeratorID: action.Moderator.ModeratorID,
			PublicKey:   action.Moderator.PublicKey,
			Signature:   action.Signature,
			SignedAt:    action.CreatedAt,
		},
	}

	return s.redisManager.PublishModerationEvent(ctx, event)
}

// BanContent processa um banimento de conteúdo
func (s *ModerationService) BanContent(ctx context.Context, action *domain.ModerationAction, category string) error {
	// Verifica se é um moderador válido
	if !s.verifier.IsMasterKey(action.Moderator.PublicKey) {
		return ErrUnauthorizedModerator
	}

	// Serializa e verifica assinatura
	actionData, err := s.serializeActionForSigning(action)
	if err != nil {
		return err
	}

	if err := s.verifier.VerifyMasterSignature(
		action.Moderator.PublicKey,
		actionData,
		action.Signature,
	); err != nil {
		return ErrInvalidModerationSign
	}

	// Cria o ban de conteúdo
	ban := &domain.ContentBan{
		ContentHash: action.TargetID,
		Reason:      action.Reason,
		Category:    category,
		BannedAt:    time.Now(),
		ModeratorID: action.Moderator.ModeratorID,
	}

	// Persiste
	if err := s.pgRepo.SaveContentBan(ctx, ban); err != nil {
		return err
	}

	// Salva a ação
	s.pgRepo.SaveModerationAction(ctx, action)

	// Propaga evento
	event := &domain.ModerationEvent{
		EventID:   generateEventID(),
		Type:      domain.EventTypeContentBanned,
		Timestamp: time.Now(),
		Payload:   ban,
		Signature: &domain.ModeratorSignature{
			ModeratorID: action.Moderator.ModeratorID,
			PublicKey:   action.Moderator.PublicKey,
			Signature:   action.Signature,
			SignedAt:    action.CreatedAt,
		},
	}

	return s.redisManager.PublishModerationEvent(ctx, event)
}

// PublishGlobalAlert publica um alerta global
func (s *ModerationService) PublishGlobalAlert(ctx context.Context, action *domain.ModerationAction, alert *domain.GlobalAlert) error {
	// Verifica moderador
	if !s.verifier.IsMasterKey(action.Moderator.PublicKey) {
		return ErrUnauthorizedModerator
	}

	// Verifica assinatura
	actionData, err := s.serializeActionForSigning(action)
	if err != nil {
		return err
	}

	if err := s.verifier.VerifyMasterSignature(
		action.Moderator.PublicKey,
		actionData,
		action.Signature,
	); err != nil {
		return ErrInvalidModerationSign
	}

	// Propaga evento
	return s.PublishInternalAlert(ctx, alert, action.Moderator.ModeratorID, action.Signature, action.CreatedAt)
}

// PublishInternalAlert envia um alerta sem validar assinatura de chave mestre (para uso interno do AdminServer)
func (s *ModerationService) PublishInternalAlert(ctx context.Context, alert *domain.GlobalAlert, moderatorID string, signature []byte, signedAt time.Time) error {
	event := &domain.ModerationEvent{
		EventID:   alert.AlertID,
		Type:      domain.EventTypeGlobalAlert,
		Timestamp: time.Now(),
		Payload:   alert,
		Signature: &domain.ModeratorSignature{
			ModeratorID: moderatorID,
			Signature:   signature,
			SignedAt:    signedAt,
		},
	}

	return s.redisManager.PublishModerationEvent(ctx, event)
}

// SubscribeToEvents retorna um canal de eventos de moderação
func (s *ModerationService) SubscribeToEvents(ctx context.Context, eventTypes []domain.EventType) (<-chan *domain.ModerationEvent, error) {
	allEvents, err := s.redisManager.SubscribeToModerationEvents(ctx)
	if err != nil {
		return nil, err
	}

	// Se não há filtro, retorna todos os eventos
	if len(eventTypes) == 0 {
		return allEvents, nil
	}

	// Cria um filtro
	typeSet := make(map[domain.EventType]bool)
	for _, t := range eventTypes {
		typeSet[t] = true
	}

	filteredChan := make(chan *domain.ModerationEvent, 100)

	go func() {
		defer close(filteredChan)
		for event := range allEvents {
			if typeSet[event.Type] {
				select {
				case filteredChan <- event:
				case <-ctx.Done():
					return
				}
			}
		}
	}()

	return filteredChan, nil
}

// IsNodeBanned verifica se um nó está banido
func (s *ModerationService) IsNodeBanned(ctx context.Context, nodeID string) (bool, string, error) {
	return s.pgRepo.IsNodeBanned(ctx, nodeID)
}

// IsContentBanned verifica se um conteúdo está banido
func (s *ModerationService) IsContentBanned(ctx context.Context, contentHash string) (bool, error) {
	return s.pgRepo.IsContentBanned(ctx, contentHash)
}

// serializeActionForSigning serializa uma ação para verificação de assinatura
func (s *ModerationService) serializeActionForSigning(action *domain.ModerationAction) ([]byte, error) {
	// Cria uma cópia sem a assinatura para serialização
	toSign := struct {
		ActionID  string `json:"action_id"`
		Type      int    `json:"type"`
		TargetID  string `json:"target_id"`
		Reason    string `json:"reason"`
		Duration  int64  `json:"duration"`
		CreatedAt int64  `json:"created_at"`
	}{
		ActionID:  action.ActionID,
		Type:      int(action.Type),
		TargetID:  action.TargetID,
		Reason:    action.Reason,
		Duration:  int64(action.Duration.Seconds()),
		CreatedAt: action.CreatedAt.Unix(),
	}

	return json.Marshal(toSign)
}

func generateEventID() string {
	return uuid.New().String()
}
