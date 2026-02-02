package relay

import (
	"context"
	"fmt"
	"io"
	"log"
	"net"
	"sync"

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

	// Registra o servidor (usando interface customizada pois não temos o grpc gerado)
	// Em produção, usaríamos: pbv1.RegisterRelayServiceServer(rs.grpcServer, rs)

	log.Printf("[RelayServer] Starting on port %d", rs.port)

	return rs.grpcServer.Serve(lis)
}

// Stop para o servidor gracefully
func (rs *RelayServer) Stop() {
	if rs.grpcServer != nil {
		rs.grpcServer.GracefulStop()
	}
}

// StreamMessages implementa o stream bidirecional de mensagens
// Esta função é chamada quando um nó conecta ao relay
func (rs *RelayServer) StreamMessages(nodeID string, publicKey []byte, recvChan <-chan *pbv1.Envelope, sendChan chan<- *pbv1.Envelope) error {
	// Registra a sessão
	session, err := rs.sessionManager.RegisterSession(nodeID, publicKey)
	if err != nil {
		return fmt.Errorf("failed to register session: %w", err)
	}
	defer rs.sessionManager.UnregisterSession(nodeID)

	log.Printf("[RelayServer] Node %s connected (session: %s)", nodeID[:8], session.SessionID[:8])

	var wg sync.WaitGroup
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Goroutine para receber mensagens do nó e encaminhar
	wg.Add(1)
	go func() {
		defer wg.Done()
		defer cancel()

		for {
			select {
			case <-ctx.Done():
				return
			case envelope, ok := <-recvChan:
				if !ok {
					return
				}

				// Encaminha para o destinatário
				if err := rs.sessionManager.ForwardEnvelope(envelope); err != nil {
					log.Printf("[RelayServer] Failed to forward: %v", err)

					// Envia recibo de erro de volta ao remetente
					receipt := &pbv1.Envelope{
						MessageId:    envelope.MessageId,
						SenderNodeId: "relay",
						TargetNodeId: envelope.SenderNodeId,
						MessageType:  pbv1.MessageType_MESSAGE_TYPE_ACK,
						Timestamp:    timestamppb.Now(),
						// encrypted_payload seria o DeliveryReceipt serializado
					}
					select {
					case sendChan <- receipt:
					default:
					}
				}
			}
		}
	}()

	// Goroutine para enviar mensagens para o nó
	wg.Add(1)
	go func() {
		defer wg.Done()
		defer cancel()

		for {
			select {
			case <-ctx.Done():
				return
			case envelope, ok := <-session.SendChan:
				if !ok {
					return
				}

				select {
				case sendChan <- envelope:
				default:
					log.Printf("[RelayServer] Failed to send to node %s (buffer full)", nodeID[:8])
				}
			}
		}
	}()

	wg.Wait()
	log.Printf("[RelayServer] Node %s disconnected", nodeID[:8])

	return nil
}

// HandleBidirectionalStream é uma versão simplificada para teste sem gRPC gerado
func (rs *RelayServer) HandleBidirectionalStream(
	nodeID string,
	publicKey []byte,
	stream interface {
		Send(*pbv1.Envelope) error
		Recv() (*pbv1.Envelope, error)
	},
) error {
	// Registra a sessão
	session, err := rs.sessionManager.RegisterSession(nodeID, publicKey)
	if err != nil {
		return fmt.Errorf("failed to register session: %w", err)
	}
	defer rs.sessionManager.UnregisterSession(nodeID)

	log.Printf("[RelayServer] Node %s connected via stream", nodeID[:8])

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var wg sync.WaitGroup

	// Goroutine para receber do stream
	wg.Add(1)
	go func() {
		defer wg.Done()
		defer cancel()

		for {
			envelope, err := stream.Recv()
			if err == io.EOF {
				return
			}
			if err != nil {
				log.Printf("[RelayServer] Recv error: %v", err)
				return
			}

			// Encaminha
			if err := rs.sessionManager.ForwardEnvelope(envelope); err != nil {
				log.Printf("[RelayServer] Forward failed: %v", err)
			}
		}
	}()

	// Goroutine para enviar ao stream
	wg.Add(1)
	go func() {
		defer wg.Done()
		defer cancel()

		for {
			select {
			case <-ctx.Done():
				return
			case envelope, ok := <-session.SendChan:
				if !ok {
					return
				}
				if err := stream.Send(envelope); err != nil {
					log.Printf("[RelayServer] Send error: %v", err)
					return
				}
			}
		}
	}()

	wg.Wait()
	return nil
}
