package main

import (
	"bufio"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/types/known/timestamppb"

	pbv1 "github.com/goautomatik/core-server/pkg/pb/v1"
	"github.com/goautomatik/core-server/pkg/relay"
)

const (
	identityFile    = "identity.key"
	defaultCoreAddr = "localhost:50051"
)

// Client representa o Desktop Client P2P
type Client struct {
	nodeID     string
	privateKey ed25519.PrivateKey
	publicKey  ed25519.PublicKey

	coreAddr   string
	coreClient pbv1.NetworkServiceClient
	coreConn   *grpc.ClientConn

	relayAddr   string
	relayClient pbv1.NetworkServiceClient
	relayConn   *grpc.ClientConn

	sessionManager *relay.SessionManager

	ctx    context.Context
	cancel context.CancelFunc

	// Callbacks
	onMessage func(from, message string)
}

func main() {
	log.SetFlags(log.LstdFlags | log.Lmicroseconds)
	fmt.Println("╔══════════════════════════════════════════════════════════╗")
	fmt.Println("║          🐝 ENXAME DESKTOP CLIENT (MVP)                   ║")
	fmt.Println("╚══════════════════════════════════════════════════════════╝")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Carrega ou gera identidade
	client, err := NewClient(ctx, defaultCoreAddr)
	if err != nil {
		log.Fatalf("Failed to create client: %v", err)
	}
	defer client.Close()

	fmt.Printf("\n🔑 Your Node ID: %s\n", client.nodeID)
	fmt.Println("   (Share this ID with others to receive messages)")

	// Conecta ao Core Server
	if err := client.ConnectToCore(); err != nil {
		log.Fatalf("Failed to connect to Core Server: %v", err)
	}

	// Registra no Core Server
	if err := client.Register(); err != nil {
		log.Fatalf("Failed to register: %v", err)
	}

	// Obtém relays ativos
	relays, err := client.GetActiveRelays()
	if err != nil {
		log.Printf("⚠️ Failed to get relays: %v", err)
	} else {
		fmt.Printf("\n📡 Active Relays: %d\n", len(relays))
		for i, r := range relays {
			if i < 3 {
				fmt.Printf("   %d. %s (%s)\n", i+1, r.Identity.NodeId[:16], r.Region)
			}
		}
	}

	// Inicia listener de mensagens
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		client.ListenForMessages()
	}()

	// Interface CLI
	fmt.Println("\n📝 Commands:")
	fmt.Println("   /to <NodeID> <message>  - Send message")
	fmt.Println("   /id                     - Show your Node ID")
	fmt.Println("   /grid                   - Simulate grid job")
	fmt.Println("   /quit                   - Exit")
	fmt.Println()

	// Captura sinais de shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Loop de input
	go func() {
		scanner := bufio.NewScanner(os.Stdin)
		fmt.Print("> ")
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if line == "" {
				fmt.Print("> ")
				continue
			}

			// Processa comando
			if strings.HasPrefix(line, "/") {
				client.HandleCommand(line)
			} else {
				fmt.Println("💡 Use /to <NodeID> <message> to send")
			}
			fmt.Print("> ")
		}
	}()

	// Aguarda sinal de shutdown
	<-sigChan
	fmt.Println("\n👋 Shutting down...")
	cancel()
	wg.Wait()
}

// NewClient cria um novo cliente
func NewClient(ctx context.Context, coreAddr string) (*Client, error) {
	privKey, pubKey, nodeID, err := loadOrGenerateIdentity(identityFile)
	if err != nil {
		return nil, fmt.Errorf("failed to load identity: %w", err)
	}

	clientCtx, cancel := context.WithCancel(ctx)

	return &Client{
		nodeID:         nodeID,
		privateKey:     privKey,
		publicKey:      pubKey,
		coreAddr:       coreAddr,
		ctx:            clientCtx,
		cancel:         cancel,
		sessionManager: relay.NewSessionManager(),
		onMessage:      defaultMessageHandler,
	}, nil
}

// loadOrGenerateIdentity carrega ou gera identidade Ed25519
func loadOrGenerateIdentity(path string) (ed25519.PrivateKey, ed25519.PublicKey, string, error) {
	// Tenta carregar
	if data, err := os.ReadFile(path); err == nil && len(data) == ed25519.PrivateKeySize {
		priv := ed25519.PrivateKey(data)
		pub := priv.Public().(ed25519.PublicKey)
		nodeID := deriveNodeID(pub)
		fmt.Printf("📂 Loaded identity from %s\n", path)
		return priv, pub, nodeID, nil
	}

	// Gera novo par
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, nil, "", err
	}

	// Salva
	if err := os.WriteFile(path, priv, 0600); err != nil {
		return nil, nil, "", err
	}

	nodeID := deriveNodeID(pub)
	fmt.Printf("🆕 Generated new identity, saved to %s\n", path)
	return priv, pub, nodeID, nil
}

// deriveNodeID deriva o NodeID da chave pública
func deriveNodeID(pub ed25519.PublicKey) string {
	hash := sha256.Sum256(pub)
	return hex.EncodeToString(hash[:16])
}

// ConnectToCore conecta ao Core Server
func (c *Client) ConnectToCore() error {
	conn, err := grpc.NewClient(c.coreAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return err
	}
	c.coreConn = conn
	c.coreClient = pbv1.NewNetworkServiceClient(conn)
	fmt.Printf("✅ Connected to Core Server at %s\n", c.coreAddr)
	return nil
}

// Register registra o nó no Core Server
func (c *Client) Register() error {
	signature := ed25519.Sign(c.privateKey, []byte(c.nodeID))

	req := &pbv1.RegisterNodeRequest{
		Identity: &pbv1.NodeIdentity{
			NodeId:    c.nodeID,
			PublicKey: c.publicKey,
			Signature: signature,
		},
		Type:         pbv1.NodeType_NODE_TYPE_DESKTOP,
		Endpoints:    []string{"localhost:0"}, // Dinâmico
		Version:      "1.0.0-mvp",
		Capabilities: []string{"desktop", "grid-worker"},
		Region:       "default",
	}

	resp, err := c.coreClient.RegisterNode(c.ctx, req)
	if err != nil {
		return err
	}

	if !resp.Success {
		return fmt.Errorf("registration failed: %s", resp.Message)
	}

	fmt.Println("✅ Registered with Core Server")
	return nil
}

// GetActiveRelays obtém lista de relays ativos
func (c *Client) GetActiveRelays() ([]*pbv1.NodeInfo, error) {
	resp, err := c.coreClient.GetActiveRelays(c.ctx, &pbv1.GetActiveRelaysRequest{
		Limit: 5,
	})
	if err != nil {
		return nil, err
	}
	return resp.Relays, nil
}

// ListenForMessages escuta mensagens do Relay
func (c *Client) ListenForMessages() {
	// Registra sessão local para receber mensagens
	session, err := c.sessionManager.RegisterSession(c.nodeID, c.publicKey)
	if err != nil {
		log.Printf("Failed to register session: %v", err)
		return
	}

	for {
		select {
		case <-c.ctx.Done():
			return
		case envelope := <-session.SendChan:
			// Mensagem recebida!
			msg := string(envelope.EncryptedPayload) // Simplificado para MVP
			fmt.Printf("\n📨 [%s]: %s\n> ", envelope.SenderNodeId[:16], msg)
		}
	}
}

// HandleCommand processa comandos CLI
func (c *Client) HandleCommand(line string) {
	parts := strings.Fields(line)
	if len(parts) == 0 {
		return
	}

	switch parts[0] {
	case "/to":
		if len(parts) < 3 {
			fmt.Println("Usage: /to <NodeID> <message>")
			return
		}
		targetID := parts[1]
		message := strings.Join(parts[2:], " ")
		c.SendMessage(targetID, message)

	case "/id":
		fmt.Printf("🔑 Your Node ID: %s\n", c.nodeID)

	case "/grid":
		c.SimulateGridJob()

	case "/quit", "/exit":
		fmt.Println("Bye!")
		os.Exit(0)

	default:
		fmt.Printf("Unknown command: %s\n", parts[0])
	}
}

// SendMessage envia uma mensagem para outro nó
func (c *Client) SendMessage(targetID, message string) {
	// Cria envelope
	envelope := &pbv1.Envelope{
		MessageId:        fmt.Sprintf("msg-%d", time.Now().UnixNano()),
		SenderNodeId:     c.nodeID,
		TargetNodeId:     targetID,
		EncryptedPayload: []byte(message), // Simplificado para MVP
		Nonce:            []byte(fmt.Sprintf("nonce-%d", time.Now().UnixNano())),
		SenderSignature:  ed25519.Sign(c.privateKey, []byte(message)),
		Timestamp:        timestamppb.Now(),
		MessageType:      pbv1.MessageType_MESSAGE_TYPE_CHAT,
		TtlSeconds:       300,
		Priority:         pbv1.Priority_PRIORITY_NORMAL,
	}

	// Tenta encaminhar via SessionManager local (para teste)
	err := c.sessionManager.ForwardEnvelope(envelope)
	if err != nil {
		// Em produção, enviaria para o Relay remoto
		fmt.Printf("📤 Message queued for %s (relay delivery pending)\n", targetID[:16])
	} else {
		fmt.Printf("✅ Message sent to %s\n", targetID[:16])
	}
}

// SimulateGridJob simula processamento de grid job
func (c *Client) SimulateGridJob() {
	fmt.Println("⚙️ Processando tarefa de grid (1% CPU)...")
	time.Sleep(100 * time.Millisecond)
	fmt.Println("✅ Tarefa concluída! Respondendo ao Core...")
}

// Close fecha todas as conexões
func (c *Client) Close() {
	c.cancel()
	if c.coreConn != nil {
		c.coreConn.Close()
	}
	if c.relayConn != nil {
		c.relayConn.Close()
	}
}

// defaultMessageHandler é o handler padrão de mensagens
func defaultMessageHandler(from, message string) {
	fmt.Printf("\n📨 [%s]: %s\n> ", from, message)
}
