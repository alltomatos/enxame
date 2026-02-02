package main

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	pbv1 "github.com/goautomatik/core-server/pkg/pb/v1"
)

// SimulatedRelay representa um relay simulado
type SimulatedRelay struct {
	ID         int
	NodeID     string
	PrivateKey ed25519.PrivateKey
	PublicKey  ed25519.PublicKey
	Client     pbv1.NetworkServiceClient
	Conn       *grpc.ClientConn
}

func main() {
	// Flags de configuração
	coreAddress := flag.String("core", "localhost:50051", "Core Server address")
	numRelays := flag.Int("relays", 5, "Number of simulated relays")
	heartbeatInterval := flag.Int("interval", 10, "Heartbeat interval in seconds")
	testGetRelays := flag.Bool("test-get-relays", true, "Test GetActiveRelays shuffle")
	flag.Parse()

	log.SetFlags(log.LstdFlags | log.Lmicroseconds)
	log.Printf("🚀 Enxame Load Simulator")
	log.Printf("Core Server: %s", *coreAddress)
	log.Printf("Simulated Relays: %d", *numRelays)
	log.Printf("Heartbeat Interval: %ds", *heartbeatInterval)

	// Cria contexto com cancelamento
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Cria relays simulados
	relays := make([]*SimulatedRelay, *numRelays)
	for i := 0; i < *numRelays; i++ {
		relay, err := newSimulatedRelay(i+1, *coreAddress)
		if err != nil {
			log.Fatalf("Failed to create relay %d: %v", i+1, err)
		}
		relays[i] = relay
		log.Printf("✅ Relay %d created: NodeID=%s", i+1, relay.NodeID[:16]+"...")
	}

	// Registra todos os relays
	log.Printf("\n📝 Registering %d relays...", *numRelays)
	var wg sync.WaitGroup
	for _, relay := range relays {
		wg.Add(1)
		go func(r *SimulatedRelay) {
			defer wg.Done()
			if err := r.Register(ctx); err != nil {
				log.Printf("❌ Relay %d failed to register: %v", r.ID, err)
			} else {
				log.Printf("✅ Relay %d registered successfully", r.ID)
			}
		}(relay)
	}
	wg.Wait()

	// Aguarda um pouco para os registros serem processados
	time.Sleep(2 * time.Second)

	// Testa GetActiveRelays
	if *testGetRelays {
		log.Printf("\n🔀 Testing GetActiveRelays shuffle...")
		testGetActiveRelays(ctx, relays[0].Client, 5)
	}

	// Inicia heartbeats em paralelo
	log.Printf("\n💓 Starting heartbeat loops for %d relays...", *numRelays)
	for _, relay := range relays {
		go relay.HeartbeatLoop(ctx, time.Duration(*heartbeatInterval)*time.Second)
	}

	// Aguarda sinal de shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Loop de monitoramento
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-sigChan:
			log.Println("\n🛑 Shutting down...")
			cancel()
			for _, relay := range relays {
				relay.Close()
			}
			return
		case <-ticker.C:
			if *testGetRelays {
				testGetActiveRelays(ctx, relays[0].Client, 3)
			}
		}
	}
}

// newSimulatedRelay cria um novo relay simulado
func newSimulatedRelay(id int, coreAddress string) (*SimulatedRelay, error) {
	// Gera par de chaves
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("failed to generate keys: %w", err)
	}

	// Deriva NodeID
	hash := sha256.Sum256(pub)
	nodeID := hex.EncodeToString(hash[:16])

	// Conecta ao Core Server
	conn, err := grpc.NewClient(coreAddress, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, fmt.Errorf("failed to connect: %w", err)
	}

	client := pbv1.NewNetworkServiceClient(conn)

	return &SimulatedRelay{
		ID:         id,
		NodeID:     nodeID,
		PrivateKey: priv,
		PublicKey:  pub,
		Client:     client,
		Conn:       conn,
	}, nil
}

// Register registra o relay no Core Server
func (r *SimulatedRelay) Register(ctx context.Context) error {
	// Assina o NodeID
	signature := ed25519.Sign(r.PrivateKey, []byte(r.NodeID))

	req := &pbv1.RegisterNodeRequest{
		Identity: &pbv1.NodeIdentity{
			NodeId:    r.NodeID,
			PublicKey: r.PublicKey,
			Signature: signature,
		},
		Type:         pbv1.NodeType_NODE_TYPE_RELAY,
		Endpoints:    []string{fmt.Sprintf("127.0.0.%d:5005%d", r.ID, r.ID)},
		Version:      "1.0.0-sim",
		Capabilities: []string{"relay", "loadsim"},
		Region:       fmt.Sprintf("region-%d", (r.ID%3)+1), // Distribui em 3 regiões
	}

	resp, err := r.Client.RegisterNode(ctx, req)
	if err != nil {
		return err
	}

	if !resp.Success {
		return fmt.Errorf("registration failed: %s", resp.Message)
	}

	return nil
}

// HeartbeatLoop executa o loop de heartbeat
func (r *SimulatedRelay) HeartbeatLoop(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	heartbeatCount := 0

	for {
		select {
		case <-ctx.Done():
			log.Printf("[Relay %d] Heartbeat loop stopped after %d heartbeats", r.ID, heartbeatCount)
			return
		case <-ticker.C:
			if err := r.sendHeartbeat(ctx); err != nil {
				log.Printf("[Relay %d] ❌ Heartbeat failed: %v", r.ID, err)
			} else {
				heartbeatCount++
				log.Printf("[Relay %d] ✓ Heartbeat #%d sent", r.ID, heartbeatCount)
			}
		}
	}
}

// sendHeartbeat envia um heartbeat
func (r *SimulatedRelay) sendHeartbeat(ctx context.Context) error {
	timestamp := fmt.Sprintf("heartbeat:%d", time.Now().Unix())
	signature := ed25519.Sign(r.PrivateKey, []byte(timestamp))

	req := &pbv1.HeartbeatRequest{
		NodeId:    r.NodeID,
		Signature: signature,
		Status:    pbv1.NodeStatus_NODE_STATUS_ONLINE,
		Metrics: &pbv1.NodeMetrics{
			CpuUsage:          float32(10 + r.ID*5), // Varia CPU por relay
			MemoryUsage:       float32(20 + r.ID*3),
			ActiveConnections: int32(r.ID * 10),
		},
	}

	resp, err := r.Client.Heartbeat(ctx, req)
	if err != nil {
		return err
	}

	if !resp.Success {
		return fmt.Errorf("heartbeat not acknowledged")
	}

	return nil
}

// Close fecha a conexão
func (r *SimulatedRelay) Close() {
	if r.Conn != nil {
		r.Conn.Close()
	}
}

// testGetActiveRelays testa o shuffle dos relays
func testGetActiveRelays(ctx context.Context, client pbv1.NetworkServiceClient, iterations int) {
	log.Printf("\n📊 Testing GetActiveRelays shuffle with %d iterations:", iterations)

	// Conta quantas vezes cada relay aparece primeiro
	firstPositionCount := make(map[string]int)

	for i := 0; i < iterations; i++ {
		resp, err := client.GetActiveRelays(ctx, &pbv1.GetActiveRelaysRequest{
			Limit: 5,
		})
		if err != nil {
			log.Printf("   ❌ Iteration %d failed: %v", i+1, err)
			continue
		}

		if len(resp.Relays) == 0 {
			log.Printf("   ⚠️ Iteration %d: No relays returned", i+1)
			continue
		}

		// Registra ordem
		order := make([]string, 0, len(resp.Relays))
		for _, relay := range resp.Relays {
			shortID := relay.Identity.NodeId[:8]
			order = append(order, shortID)
		}

		// Conta primeiro da lista
		firstPositionCount[order[0]]++

		log.Printf("   Iteration %d: Order = [%s]", i+1, formatOrder(order))
	}

	// Exibe distribuição
	log.Printf("\n📈 First Position Distribution:")
	for nodeID, count := range firstPositionCount {
		percentage := float64(count) / float64(iterations) * 100
		log.Printf("   %s: %d times (%.1f%%)", nodeID, count, percentage)
	}

	// Verifica se houve shuffle
	if len(firstPositionCount) > 1 {
		log.Printf("✅ Shuffle WORKING: Different relays appeared first")
	} else if len(firstPositionCount) == 1 {
		log.Printf("⚠️ Shuffle NOT WORKING: Same relay always first (may need more iterations)")
	} else {
		log.Printf("❌ No relays returned")
	}
}

func formatOrder(order []string) string {
	result := ""
	for i, id := range order {
		if i > 0 {
			result += ", "
		}
		result += id
	}
	return result
}
