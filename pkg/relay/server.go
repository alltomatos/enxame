package relay

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/keepalive"
	"google.golang.org/protobuf/types/known/timestamppb"

	pbv1 "github.com/goautomatik/core-server/pkg/pb/v1"
)

// RelayServer implementa o serviço gRPC de relay de mensagens
type RelayServer struct {
	sessionManager *SessionManager
	grpcServer     *grpc.Server
	port           int
}

// NewRelayServer cria um novo servidor de relay
func NewRelayServer(port int, sessionManager *SessionManager) *RelayServer {
	return &RelayServer{
		sessionManager: sessionManager,
		port:           port,
	}
}

// Implementação interface RelayServiceServer
func (rs *RelayServer) ConnectToRelay(ctx context.Context, req *pbv1.ConnectRequest) (*pbv1.ConnectResponse, error) {
	// Validação básica (em prod validaríamos assinatura)
	if req.NodeId == "" {
		return &pbv1.ConnectResponse{Success: false, Message: "NodeID required"}, nil
	}

	session, err := rs.sessionManager.RegisterSession(req.NodeId, req.PublicKey)
	if err != nil {
		return &pbv1.ConnectResponse{Success: false, Message: err.Error()}, nil
	}

	return &pbv1.ConnectResponse{
		Success:          true,
		Message:          "Connected to Relay",
		SessionId:        session.SessionID,
		ConnectedNodeIds: rs.sessionManager.GetConnectedNodeIDs(),
	}, nil
}

func (rs *RelayServer) IsNodeConnected(ctx context.Context, req *pbv1.NodeQueryRequest) (*pbv1.NodeQueryResponse, error) {
	session, exists := rs.sessionManager.GetSession(req.NodeId)
	if !exists {
		return &pbv1.NodeQueryResponse{Connected: false}, nil
	}
	return &pbv1.NodeQueryResponse{
		Connected:    true,
		LastActivity: timestamppb.New(session.LastActivity),
	}, nil
}

// Start inicia o servidor gRPC do Relay
func (rs *RelayServer) Start() error {
	lis, err := net.Listen("tcp", fmt.Sprintf(":%d", rs.port))
	if err != nil {
		return fmt.Errorf("failed to listen: %w", err)
	}

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
	}

	rs.grpcServer = grpc.NewServer(opts...)

	// Registra o servidor manualmente
	RegisterRelayServiceServer(rs.grpcServer, rs)

	log.Printf("[RelayServer] Starting on port %d", rs.port)

	return rs.grpcServer.Serve(lis)
}

// Stop para o servidor gracefully
func (rs *RelayServer) Stop() {
	if rs.grpcServer != nil {
		rs.grpcServer.GracefulStop()
	}
}

// StreamMessages implementa o stream bidirecional
func (rs *RelayServer) StreamMessages(stream RelayService_StreamMessagesServer) error {
	// Lê primeira mensagem (Handshake)
	firstCmd, err := stream.Recv()
	if err != nil {
		return err
	}

	firstEnv, err := CommandToEnvelope(firstCmd)
	if err != nil {
		return err
	}

	nodeID := firstEnv.SenderNodeId
	if nodeID == "" {
		return fmt.Errorf("sender_node_id required in first message")
	}

	// Registra sessão
	session, err := rs.sessionManager.RegisterSession(nodeID, nil) // PublicKey opcional no MVP
	if err != nil {
		return err
	}
	defer rs.sessionManager.UnregisterSession(nodeID)

	log.Printf("[RelayServer] Stream started for node %s", nodeID[:8])

	// Se for mensagem real, encaminha
	if len(firstEnv.EncryptedPayload) > 0 {
		rs.sessionManager.ForwardEnvelope(firstEnv)
	}

	errChan := make(chan error, 2)
	ctx := stream.Context()

	// Goroutine de envio (Relay -> Client)
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case env := <-session.SendChan:
				cmd, err := EnvelopeToCommand(env)
				if err != nil {
					log.Printf("Failed to serialize envelope: %v", err)
					continue
				}
				if err := stream.Send(cmd); err != nil {
					errChan <- err
					return
				}
			}
		}
	}()

	// Loop de recebimento (Client -> Relay)
	for {
		cmd, err := stream.Recv()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}

		envelope, err := CommandToEnvelope(cmd)
		if err != nil {
			log.Printf("Invalid command received: %v", err)
			continue
		}
		if envelope == nil {
			continue
		}

		// Validação básica
		if envelope.SenderNodeId != nodeID {
			log.Printf("[RelayServer] spoofing attempt? claimed %s but stream is %s", envelope.SenderNodeId, nodeID)
			continue
		}

		session.LastActivity = time.Now()

		// INTERCEPTA COMANDOS PARA O RELAY (Control Plane)
		if envelope.TargetNodeId == "RELAY" {
			rs.handleRelayCommand(session, envelope)
			continue
		}

		if err := rs.sessionManager.ForwardEnvelope(envelope); err != nil {
			log.Printf("[RelayServer] Forward error: %v", err)
		}
	}
}

// handleRelayCommand processa comandos administrativos enviados ao Relay
func (rs *RelayServer) handleRelayCommand(session *Session, envelope *pbv1.Envelope) {
	// Payload esperado: JSON {"command": "subscribe", "channel": "#foo"}
	// Como é comando para o Relay, o payload não é cifrado E2E, apenas assinado (já validado)
	// Formato simples JSON
	type RelayCommand struct {
		Command string `json:"command"`
		Channel string `json:"channel"`
	}

	var cmd RelayCommand
	// No MVP, assumimos que EncryptedPayload contém o JSON puro se target=RELAY
	// TODO: Na versão final, usar E2EE com a chave do Relay (se disponível)
	// Para simplificar, vamos tentar fazer Unmarshal direto.
	// Dica: O cliente deve enviar apenas o JSON bytes no EncryptedPayload para este caso.

	// Pequeno hack: o pbv1.Envelope espera []byte em EncryptedPayload.
	// Vamos varrer o JSON dali.
	if err := json.Unmarshal(envelope.EncryptedPayload, &cmd); err != nil {
		log.Printf("[RelayServer] Invalid control command from %s: %v", session.NodeID[:8], err)
		return
	}

	switch cmd.Command {
	case "subscribe":
		if len(cmd.Channel) > 0 && cmd.Channel[0] == '#' {
			rs.sessionManager.SubscribeToChannel(session.NodeID, cmd.Channel)
		}
	case "unsubscribe":
		rs.sessionManager.UnsubscribeFromChannel(session.NodeID, cmd.Channel)
	default:
		log.Printf("[RelayServer] Unknown control command from %s: %s", session.NodeID[:8], cmd.Command)
	}
}
