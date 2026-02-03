package relay

import (
	"context"
	"crypto/ed25519"
	"fmt"
	"io"
	"log"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	pbv1 "github.com/goautomatik/core-server/pkg/pb/v1"
)

// RelayClient gerencia conexão e streaming com um Relay Server
type RelayClient struct {
	conn         *grpc.ClientConn
	stream       RelayService_StreamMessagesClient
	nodeID       string
	publicKey    ed25519.PublicKey
	connected    bool
	currentRelay string
}

// RelayService_StreamMessagesClient interface manual para o cliente stream
type RelayService_StreamMessagesClient interface {
	Send(*pbv1.NodeCommand) error
	Recv() (*pbv1.NodeCommand, error)
	grpc.ClientStream
}

// streamWrapper implementa RelayService_StreamMessagesClient
type streamWrapper struct {
	grpc.ClientStream
}

func (x *streamWrapper) Send(m *pbv1.NodeCommand) error {
	return x.ClientStream.SendMsg(m)
}

func (x *streamWrapper) Recv() (*pbv1.NodeCommand, error) {
	m := new(pbv1.NodeCommand)
	if err := x.ClientStream.RecvMsg(m); err != nil {
		return nil, err
	}
	return m, nil
}

// NewRelayClient cria um novo cliente
func NewRelayClient(nodeID string, publicKey ed25519.PublicKey) *RelayClient {
	return &RelayClient{
		nodeID:    nodeID,
		publicKey: publicKey,
	}
}

// Connect conecta a um Relay específico
func (c *RelayClient) Connect(ctx context.Context, endpoint string) error {
	conn, err := grpc.NewClient(endpoint, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return fmt.Errorf("failed to connect grpc: %w", err)
	}

	c.conn = conn
	c.currentRelay = endpoint

	// Inicia stream manual
	streamDesc := &grpc.StreamDesc{
		StreamName:    "StreamMessages",
		ServerStreams: true,
		ClientStreams: true,
	}

	stream, err := conn.NewStream(ctx, streamDesc, "/relay.v1.RelayService/StreamMessages")
	if err != nil {
		conn.Close()
		return fmt.Errorf("failed to create stream: %w", err)
	}

	c.stream = &streamWrapper{ClientStream: stream}
	c.connected = true // Marca como conectado para permitir o envio do handshake

	// Envia handshake (primeira mensagem)
	// Como envelopamos em NodeCommand, enviamos um envelope vazio de handshake
	handshake := &pbv1.Envelope{
		MessageId:    "handshake",
		SenderNodeId: c.nodeID,
		MessageType:  pbv1.MessageType_MESSAGE_TYPE_CONTROL,
	}
	if err := c.Send(handshake); err != nil {
		c.connected = false
		conn.Close()
		return fmt.Errorf("handshake failed: %w", err)
	}

	log.Printf("[RelayClient] Connected to %s", endpoint)
	return nil
}

// Send envia um envelope para o stream
func (c *RelayClient) Send(envelope *pbv1.Envelope) error {
	if !c.connected || c.stream == nil {
		return fmt.Errorf("not connected to relay")
	}

	cmd, err := EnvelopeToCommand(envelope)
	if err != nil {
		return err
	}

	return c.stream.Send(cmd)
}

// ReceiveLoop processa mensagens recebidas
func (c *RelayClient) ReceiveLoop(handler func(*pbv1.Envelope)) error {
	if !c.connected || c.stream == nil {
		return fmt.Errorf("not connected to relay")
	}

	for {
		cmd, err := c.stream.Recv()
		if err == io.EOF {
			c.connected = false
			return fmt.Errorf("stream closed by server")
		}
		if err != nil {
			c.connected = false
			return fmt.Errorf("stream error: %w", err)
		}

		envelope, err := CommandToEnvelope(cmd)
		if err != nil {
			log.Printf("Invalid envelope received: %v", err)
			continue
		}
		if envelope == nil {
			continue
		}

		// Chama callback
		if handler != nil {
			handler(envelope)
		}
	}
}

// Close fecha conexão
func (c *RelayClient) Close() {
	if c.conn != nil {
		c.conn.Close()
	}
	c.connected = false
}

// IsConnected retorna status
func (c *RelayClient) IsConnected() bool {
	return c.connected
}
