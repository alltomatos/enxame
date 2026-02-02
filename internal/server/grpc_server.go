package server

import (
	"context"
	"log"
	"net"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/keepalive"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/goautomatik/core-server/internal/config"
	"github.com/goautomatik/core-server/internal/domain"
	"github.com/goautomatik/core-server/internal/service"
	pbv1 "github.com/goautomatik/core-server/pkg/pb/v1"
)

// GRPCServer implementa o NetworkServiceServer
type GRPCServer struct {
	pbv1.UnimplementedNetworkServiceServer

	cfg             *config.Config
	nodeService     *service.NodeService
	modService      *service.ModerationService
	grpcServer      *grpc.Server
	authInterceptor *AuthInterceptor

	// Gerenciamento de streams ativos com sharding
	streamManager *StreamManager
}

// NewGRPCServer cria um novo servidor gRPC
func NewGRPCServer(
	cfg *config.Config,
	nodeService *service.NodeService,
	modService *service.ModerationService,
	authInterceptor *AuthInterceptor,
) *GRPCServer {
	return &GRPCServer{
		cfg:             cfg,
		nodeService:     nodeService,
		modService:      modService,
		authInterceptor: authInterceptor,
		streamManager:   NewStreamManager(100), // Buffer de 100 eventos por stream
	}
}

// Start inicia o servidor gRPC
func (s *GRPCServer) Start(port string) error {
	lis, err := net.Listen("tcp", ":"+port)
	if err != nil {
		return err
	}

	// Configurações otimizadas para alta concorrência
	opts := []grpc.ServerOption{
		grpc.MaxConcurrentStreams(10000),
		grpc.MaxRecvMsgSize(4 * 1024 * 1024), // 4MB
		grpc.MaxSendMsgSize(4 * 1024 * 1024),
		grpc.KeepaliveParams(keepalive.ServerParameters{
			MaxConnectionIdle:     5 * time.Minute,
			MaxConnectionAge:      30 * time.Minute,
			MaxConnectionAgeGrace: 5 * time.Second,
			Time:                  1 * time.Minute,
			Timeout:               20 * time.Second,
		}),
		grpc.KeepaliveEnforcementPolicy(keepalive.EnforcementPolicy{
			MinTime:             10 * time.Second,
			PermitWithoutStream: true,
		}),
		grpc.ChainUnaryInterceptor(
			RecoveryInterceptor,
			LoggingInterceptor,
			s.authInterceptor.Unary(), // Interceptor de autenticação Ed25519
		),
		grpc.ChainStreamInterceptor(
			StreamRecoveryInterceptor,
			StreamLoggingInterceptor,
			s.authInterceptor.Stream(),
		),
	}

	s.grpcServer = grpc.NewServer(opts...)
	pbv1.RegisterNetworkServiceServer(s.grpcServer, s)

	log.Printf("gRPC server starting on port %s", port)

	// Inicia goroutine para propagar eventos de moderação
	go s.startEventBroadcaster()

	return s.grpcServer.Serve(lis)
}

// Stop para o servidor gracefully
func (s *GRPCServer) Stop() {
	if s.grpcServer != nil {
		s.grpcServer.GracefulStop()
	}

	// Fecha todos os streams ativos (StreamManager lida com isso)
	// O StreamManager não precisa de reset, as goroutines tratam o fechamento
}

// RegisterNode implementa o RPC de registro de nó
func (s *GRPCServer) RegisterNode(ctx context.Context, req *pbv1.RegisterNodeRequest) (*pbv1.RegisterNodeResponse, error) {
	// Converte para domain
	reg := &domain.NodeRegistration{
		Identity: domain.NodeIdentity{
			NodeID:    req.Identity.NodeId,
			PublicKey: req.Identity.PublicKey,
			Signature: req.Identity.Signature,
		},
		Type:         pbNodeTypeToDomain(req.Type),
		Endpoints:    req.Endpoints,
		Version:      req.Version,
		Capabilities: req.Capabilities,
		Region:       req.Region,
	}

	// Registra o nó
	node, suggestedRelays, err := s.nodeService.RegisterNode(ctx, reg)
	if err != nil {
		return &pbv1.RegisterNodeResponse{
			Success: false,
			Message: err.Error(),
		}, nil
	}

	log.Printf("Node registered: %s (type: %s)", node.Identity.NodeID, node.Type.String())

	// Converte relays sugeridos
	pbRelays := make([]*pbv1.NodeInfo, 0, len(suggestedRelays))
	for _, relay := range suggestedRelays {
		pbRelays = append(pbRelays, domainNodeToPb(relay))
	}

	return &pbv1.RegisterNodeResponse{
		Success:                  true,
		Message:                  "Node registered successfully",
		HeartbeatIntervalSeconds: int32(s.cfg.HeartbeatInterval.Seconds()),
		SuggestedRelays:          pbRelays,
	}, nil
}

// Heartbeat implementa o RPC de heartbeat
func (s *GRPCServer) Heartbeat(ctx context.Context, req *pbv1.HeartbeatRequest) (*pbv1.HeartbeatResponse, error) {
	var metrics *domain.NodeMetrics
	if req.Metrics != nil {
		metrics = &domain.NodeMetrics{
			CPUUsage:               req.Metrics.CpuUsage,
			MemoryUsage:            req.Metrics.MemoryUsage,
			ActiveConnections:      req.Metrics.ActiveConnections,
			AvailableBandwidthMbps: req.Metrics.AvailableBandwidthMbps,
			LatencyMs:              req.Metrics.LatencyMs,
		}
	}

	nextHeartbeat, err := s.nodeService.Heartbeat(
		ctx,
		req.NodeId,
		req.Signature,
		pbNodeStatusToDomain(req.Status),
		metrics,
	)

	if err != nil {
		return &pbv1.HeartbeatResponse{
			Success: false,
		}, nil
	}

	return &pbv1.HeartbeatResponse{
		Success:              true,
		NextHeartbeatSeconds: int32(nextHeartbeat),
	}, nil
}

// GetActiveRelays implementa o RPC de descoberta de relays
func (s *GRPCServer) GetActiveRelays(ctx context.Context, req *pbv1.GetActiveRelaysRequest) (*pbv1.GetActiveRelaysResponse, error) {
	relays, err := s.nodeService.GetActiveRelays(
		ctx,
		req.PreferredRegion,
		int(req.Limit),
		req.ExcludeNodeIds,
	)
	if err != nil {
		return nil, err
	}

	pbRelays := make([]*pbv1.NodeInfo, 0, len(relays))
	for _, relay := range relays {
		pbRelays = append(pbRelays, domainNodeToPb(relay))
	}

	return &pbv1.GetActiveRelaysResponse{
		Relays:    pbRelays,
		UpdatedAt: timestamppb.Now(),
	}, nil
}

// SubscribeToGlobalEvents implementa o stream de eventos globais
func (s *GRPCServer) SubscribeToGlobalEvents(req *pbv1.SubscribeRequest, stream pbv1.NetworkService_SubscribeToGlobalEventsServer) error {
	// Registra stream usando StreamManager (sharded para alta concorrência)
	eventChan := s.streamManager.Register(req.NodeId)

	defer s.streamManager.Unregister(req.NodeId)

	log.Printf("Node %s subscribed to global events (total: %d)", req.NodeId, s.streamManager.Count())

	// Loop de envio de eventos
	for {
		select {
		case <-stream.Context().Done():
			log.Printf("Node %s unsubscribed from global events", req.NodeId)
			return nil
		case event, ok := <-eventChan:
			if !ok {
				return nil
			}
			if err := stream.Send(event); err != nil {
				log.Printf("Error sending event to node %s: %v", req.NodeId, err)
				return err
			}
		}
	}
}

// startEventBroadcaster inicia a goroutine que propaga eventos para todos os streams
func (s *GRPCServer) startEventBroadcaster() {
	ctx := context.Background()

	// Converte tipos de evento (todos os tipos)
	eventTypes := []domain.EventType{
		domain.EventTypeNodeBanned,
		domain.EventTypeContentBanned,
		domain.EventTypeGlobalAlert,
		domain.EventTypeRelayOffline,
		domain.EventTypeNetworkUpdate,
	}

	eventChan, err := s.modService.SubscribeToEvents(ctx, eventTypes)
	if err != nil {
		log.Printf("Failed to subscribe to moderation events: %v", err)
		return
	}

	for event := range eventChan {
		pbEvent := domainEventToPb(event)

		// Broadcast assíncrono usando StreamManager (sharded)
		s.streamManager.BroadcastAsync(pbEvent)
	}
}

// Funções auxiliares de conversão

func pbNodeTypeToDomain(t pbv1.NodeType) domain.NodeType {
	switch t {
	case pbv1.NodeType_NODE_TYPE_DESKTOP:
		return domain.NodeTypeDesktop
	case pbv1.NodeType_NODE_TYPE_RELAY:
		return domain.NodeTypeRelay
	case pbv1.NodeType_NODE_TYPE_MOBILE:
		return domain.NodeTypeMobile
	case pbv1.NodeType_NODE_TYPE_WEB:
		return domain.NodeTypeWeb
	default:
		return domain.NodeTypeUnspecified
	}
}

func pbNodeStatusToDomain(s pbv1.NodeStatus) domain.NodeStatus {
	switch s {
	case pbv1.NodeStatus_NODE_STATUS_ONLINE:
		return domain.NodeStatusOnline
	case pbv1.NodeStatus_NODE_STATUS_AWAY:
		return domain.NodeStatusAway
	case pbv1.NodeStatus_NODE_STATUS_BUSY:
		return domain.NodeStatusBusy
	case pbv1.NodeStatus_NODE_STATUS_OFFLINE:
		return domain.NodeStatusOffline
	default:
		return domain.NodeStatusUnspecified
	}
}

func domainNodeToPb(n *domain.Node) *pbv1.NodeInfo {
	return &pbv1.NodeInfo{
		Identity: &pbv1.NodeIdentity{
			NodeId:    n.Identity.NodeID,
			PublicKey: n.Identity.PublicKey,
			Signature: n.Identity.Signature,
		},
		Type:         domainNodeTypeToPb(n.Type),
		Status:       domainNodeStatusToPb(n.Status),
		Endpoints:    n.Endpoints,
		Version:      n.Version,
		Capabilities: n.Capabilities,
		Region:       n.Region,
		RegisteredAt: timestamppb.New(n.RegisteredAt),
		LastSeen:     timestamppb.New(n.LastSeen),
	}
}

func domainNodeTypeToPb(t domain.NodeType) pbv1.NodeType {
	switch t {
	case domain.NodeTypeDesktop:
		return pbv1.NodeType_NODE_TYPE_DESKTOP
	case domain.NodeTypeRelay:
		return pbv1.NodeType_NODE_TYPE_RELAY
	case domain.NodeTypeMobile:
		return pbv1.NodeType_NODE_TYPE_MOBILE
	case domain.NodeTypeWeb:
		return pbv1.NodeType_NODE_TYPE_WEB
	default:
		return pbv1.NodeType_NODE_TYPE_UNSPECIFIED
	}
}

func domainNodeStatusToPb(s domain.NodeStatus) pbv1.NodeStatus {
	switch s {
	case domain.NodeStatusOnline:
		return pbv1.NodeStatus_NODE_STATUS_ONLINE
	case domain.NodeStatusAway:
		return pbv1.NodeStatus_NODE_STATUS_AWAY
	case domain.NodeStatusBusy:
		return pbv1.NodeStatus_NODE_STATUS_BUSY
	case domain.NodeStatusOffline:
		return pbv1.NodeStatus_NODE_STATUS_OFFLINE
	default:
		return pbv1.NodeStatus_NODE_STATUS_UNSPECIFIED
	}
}

func domainEventToPb(e *domain.ModerationEvent) *pbv1.GlobalEvent {
	pbEvent := &pbv1.GlobalEvent{
		EventId:   e.EventID,
		Type:      domainEventTypeToPb(e.Type),
		Timestamp: timestamppb.New(e.Timestamp),
	}

	if e.Signature != nil {
		pbEvent.ModeratorSignature = &pbv1.ModeratorSignature{
			ModeratorId: e.Signature.ModeratorID,
			PublicKey:   e.Signature.PublicKey,
			Signature:   e.Signature.Signature,
			SignedAt:    timestamppb.New(e.Signature.SignedAt),
		}
	}

	// Converte payload baseado no tipo
	switch payload := e.Payload.(type) {
	case *domain.NodeBan:
		pbEvent.Payload = &pbv1.GlobalEvent_NodeBanned{
			NodeBanned: &pbv1.NodeBannedEvent{
				BannedNodeId:    payload.BannedNodeID,
				Reason:          payload.Reason,
				DurationSeconds: payload.DurationSeconds,
				EvidenceHash:    payload.EvidenceHash,
			},
		}
	case *domain.ContentBan:
		pbEvent.Payload = &pbv1.GlobalEvent_ContentBanned{
			ContentBanned: &pbv1.ContentBannedEvent{
				ContentHash: payload.ContentHash,
				Reason:      payload.Reason,
				Category:    payload.Category,
			},
		}
	case *domain.GlobalAlert:
		pbEvent.Payload = &pbv1.GlobalEvent_GlobalAlert{
			GlobalAlert: &pbv1.GlobalAlertEvent{
				Title:    payload.Title,
				Message:  payload.Message,
				Severity: payload.Severity,
				InfoUrl:  payload.InfoURL,
			},
		}
	}

	return pbEvent
}

func domainEventTypeToPb(t domain.EventType) pbv1.EventType {
	switch t {
	case domain.EventTypeNodeBanned:
		return pbv1.EventType_EVENT_TYPE_NODE_BANNED
	case domain.EventTypeContentBanned:
		return pbv1.EventType_EVENT_TYPE_CONTENT_BANNED
	case domain.EventTypeGlobalAlert:
		return pbv1.EventType_EVENT_TYPE_GLOBAL_ALERT
	case domain.EventTypeRelayOffline:
		return pbv1.EventType_EVENT_TYPE_RELAY_OFFLINE
	case domain.EventTypeNetworkUpdate:
		return pbv1.EventType_EVENT_TYPE_NETWORK_UPDATE
	default:
		return pbv1.EventType_EVENT_TYPE_UNSPECIFIED
	}
}
