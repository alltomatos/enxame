package client

import (
	"context"
	"fmt"
	"log"
	"runtime"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/keepalive"

	pbv1 "github.com/goautomatik/core-server/pkg/pb/v1"
	"github.com/goautomatik/relay/internal/config"
	"github.com/goautomatik/relay/internal/crypto"
)

// CoreClient é o cliente gRPC para o Core Server
type CoreClient struct {
	cfg     *config.Config
	keyPair *crypto.KeyPair
	conn    *grpc.ClientConn
	client  pbv1.NetworkServiceClient

	// Controle de ciclo de vida
	ctx    context.Context
	cancel context.CancelFunc
}

// NewCoreClient cria um novo cliente do Core Server
func NewCoreClient(cfg *config.Config, keyPair *crypto.KeyPair) *CoreClient {
	ctx, cancel := context.WithCancel(context.Background())
	return &CoreClient{
		cfg:     cfg,
		keyPair: keyPair,
		ctx:     ctx,
		cancel:  cancel,
	}
}

// Connect estabelece conexão com o Core Server
func (c *CoreClient) Connect() error {
	opts := []grpc.DialOption{
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithKeepaliveParams(keepalive.ClientParameters{
			Time:                30 * time.Second,
			Timeout:             10 * time.Second,
			PermitWithoutStream: true,
		}),
	}

	conn, err := grpc.NewClient(c.cfg.CoreAddress, opts...)
	if err != nil {
		return fmt.Errorf("failed to connect to core server: %w", err)
	}

	c.conn = conn
	c.client = pbv1.NewNetworkServiceClient(conn)

	log.Printf("[Client] Connected to Core Server at %s", c.cfg.CoreAddress)
	return nil
}

// Register registra o relay no Core Server
func (c *CoreClient) Register() error {
	// Prepara a requisição
	req := &pbv1.RegisterNodeRequest{
		Identity: &pbv1.NodeIdentity{
			NodeId:    c.keyPair.NodeID,
			PublicKey: c.keyPair.PublicKey,
			Signature: c.keyPair.SignNodeID(),
		},
		Type:         pbv1.NodeType_NODE_TYPE_RELAY,
		Endpoints:    []string{fmt.Sprintf("%s:%d", c.cfg.RelayIP, c.cfg.RelayPort)},
		Version:      "1.0.0",
		Capabilities: c.cfg.Capabilities,
		Region:       c.cfg.Region,
	}

	resp, err := c.client.RegisterNode(c.ctx, req)
	if err != nil {
		return fmt.Errorf("failed to register: %w", err)
	}

	if !resp.Success {
		return fmt.Errorf("registration failed: %s", resp.Message)
	}

	log.Printf("[Client] ✅ Registered successfully!")
	log.Printf("[Client] Heartbeat interval: %ds", resp.HeartbeatIntervalSeconds)
	log.Printf("[Client] Suggested relays: %d", len(resp.SuggestedRelays))

	return nil
}

// StartHeartbeatLoop inicia o loop de heartbeat
func (c *CoreClient) StartHeartbeatLoop() {
	ticker := time.NewTicker(time.Duration(c.cfg.HeartbeatInterval) * time.Second)
	defer ticker.Stop()

	log.Printf("[Heartbeat] Starting heartbeat loop (interval: %ds)", c.cfg.HeartbeatInterval)

	for {
		select {
		case <-c.ctx.Done():
			log.Println("[Heartbeat] Stopping heartbeat loop")
			return
		case <-ticker.C:
			if err := c.sendHeartbeat(); err != nil {
				log.Printf("[Heartbeat] ❌ Failed: %v", err)
			} else {
				log.Println("[Heartbeat] ✓ Sent successfully")
			}
		}
	}
}

// sendHeartbeat envia um heartbeat para o Core Server
func (c *CoreClient) sendHeartbeat() error {
	// Calcula métricas do sistema
	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)

	req := &pbv1.HeartbeatRequest{
		NodeId:    c.keyPair.NodeID,
		Signature: c.keyPair.Sign([]byte(fmt.Sprintf("heartbeat:%d", time.Now().Unix()))),
		Status:    pbv1.NodeStatus_NODE_STATUS_ONLINE,
		Metrics: &pbv1.NodeMetrics{
			CpuUsage:          getCPUUsage(),
			MemoryUsage:       float32(memStats.Alloc) / float32(memStats.TotalAlloc) * 100,
			ActiveConnections: 0, // TODO: implementar contagem de conexões
		},
	}

	resp, err := c.client.Heartbeat(c.ctx, req)
	if err != nil {
		return err
	}

	if !resp.Success {
		return fmt.Errorf("heartbeat not acknowledged")
	}

	return nil
}

// SubscribeToEvents inscreve-se nos eventos globais de moderação
func (c *CoreClient) SubscribeToEvents() {
	log.Println("[Events] Subscribing to global events...")

	req := &pbv1.SubscribeRequest{
		NodeId: c.keyPair.NodeID,
		EventTypes: []pbv1.EventType{
			pbv1.EventType_EVENT_TYPE_NODE_BANNED,
			pbv1.EventType_EVENT_TYPE_CONTENT_BANNED,
			pbv1.EventType_EVENT_TYPE_GLOBAL_ALERT,
		},
	}

	stream, err := c.client.SubscribeToGlobalEvents(c.ctx, req)
	if err != nil {
		log.Printf("[Events] Failed to subscribe: %v", err)
		return
	}

	log.Println("[Events] ✅ Subscribed successfully, listening for events...")

	for {
		event, err := stream.Recv()
		if err != nil {
			log.Printf("[Events] Stream error: %v", err)
			return
		}

		c.handleEvent(event)
	}
}

// handleEvent processa um evento recebido
func (c *CoreClient) handleEvent(event *pbv1.GlobalEvent) {
	switch event.Type {
	case pbv1.EventType_EVENT_TYPE_NODE_BANNED:
		log.Printf("🚫 [BAN] Node banned! EventID: %s", event.EventId)

	case pbv1.EventType_EVENT_TYPE_CONTENT_BANNED:
		log.Printf("🚫 [BAN] Content banned! EventID: %s", event.EventId)

	case pbv1.EventType_EVENT_TYPE_GLOBAL_ALERT:
		log.Printf("⚠️ [ALERT] Global alert! EventID: %s", event.EventId)

	case pbv1.EventType_EVENT_TYPE_RELAY_OFFLINE:
		log.Printf("📡 [RELAY] Relay offline! EventID: %s", event.EventId)

	case pbv1.EventType_EVENT_TYPE_NETWORK_UPDATE:
		log.Printf("🔄 [UPDATE] Network update! EventID: %s", event.EventId)

	default:
		log.Printf("[Events] Unknown event type: %v", event.Type)
	}
}

// Close fecha a conexão com o Core Server
func (c *CoreClient) Close() {
	c.cancel()
	if c.conn != nil {
		c.conn.Close()
	}
	log.Println("[Client] Connection closed")
}

// getCPUUsage retorna uma estimativa simples de uso de CPU
func getCPUUsage() float32 {
	// Simplificado - em produção, use métricas reais
	return float32(runtime.NumGoroutine()) / 100.0
}
