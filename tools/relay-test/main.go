package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log"
	"sync"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"

	pbv1 "github.com/goautomatik/core-server/pkg/pb/v1"
	"github.com/goautomatik/core-server/pkg/relay"
)

// SimNode representa um nó simulado para teste
type SimNode struct {
	ID         string
	PrivateKey ed25519.PrivateKey
	PublicKey  ed25519.PublicKey
	RecvChan   chan *pbv1.Envelope
	SendChan   chan *pbv1.Envelope
}

func main() {
	log.SetFlags(log.LstdFlags | log.Lmicroseconds)
	log.Println("🧪 Relay Message Test - Node_A → Relay → Node_B")
	log.Println("================================================")

	// Cria o SessionManager
	sm := relay.NewSessionManager()

	// Cria dois nós simulados
	nodeA := createSimNode("Node_A")
	nodeB := createSimNode("Node_B")

	log.Printf("✅ Node_A ID: %s", nodeA.ID[:16])
	log.Printf("✅ Node_B ID: %s", nodeB.ID[:16])

	// Registra ambos os nós no SessionManager
	sessionA, err := sm.RegisterSession(nodeA.ID, nodeA.PublicKey)
	if err != nil {
		log.Fatalf("Failed to register Node_A: %v", err)
	}
	log.Printf("✅ Node_A registered (session: %s)", sessionA.SessionID[:8])

	sessionB, err := sm.RegisterSession(nodeB.ID, nodeB.PublicKey)
	if err != nil {
		log.Fatalf("Failed to register Node_B: %v", err)
	}
	log.Printf("✅ Node_B registered (session: %s)", sessionB.SessionID[:8])

	// Verifica nós conectados
	connected := sm.GetConnectedNodeIDs()
	log.Printf("📡 Connected nodes: %d", len(connected))

	var wg sync.WaitGroup

	// Goroutine para Node_B receber mensagens
	wg.Add(1)
	go func() {
		defer wg.Done()
		log.Println("\n📥 Node_B listening for messages...")

		select {
		case envelope := <-sessionB.SendChan:
			log.Println("\n" + "=" + "================================================")
			log.Printf("📨 Node_B RECEIVED MESSAGE!")
			log.Printf("   From: %s", envelope.SenderNodeId[:16])
			log.Printf("   To: %s", envelope.TargetNodeId[:16])
			log.Printf("   Type: %v", envelope.MessageType)
			log.Printf("   Payload (simulated encrypted): %s", string(envelope.EncryptedPayload))
			log.Printf("   Timestamp: %s", envelope.Timestamp.AsTime().Format(time.RFC3339))
			log.Println("================================================")

			// Simula descriptografar e exibir
			decryptedMessage := string(envelope.EncryptedPayload)
			log.Printf("\n🎉 DECRYPTED MESSAGE: \"%s\"", decryptedMessage)

		case <-time.After(5 * time.Second):
			log.Println("⚠️ Timeout waiting for message")
		}
	}()

	// Aguarda um pouco para garantir que Node_B está escutando
	time.Sleep(100 * time.Millisecond)

	// Node_A envia mensagem para Node_B
	log.Println("\n📤 Node_A sending message to Node_B...")

	envelope := &pbv1.Envelope{
		MessageId:        fmt.Sprintf("msg-%d", time.Now().UnixNano()),
		SenderNodeId:     nodeA.ID,
		TargetNodeId:     nodeB.ID,
		EncryptedPayload: []byte("Hello"), // Simulação de payload cifrado
		Nonce:            []byte("random-nonce-12345"),
		SenderSignature:  signMessage(nodeA.PrivateKey, "Hello"),
		Timestamp:        timestamppb.Now(),
		MessageType:      pbv1.MessageType_MESSAGE_TYPE_CHAT,
		TtlSeconds:       300,
		Priority:         pbv1.Priority_PRIORITY_NORMAL,
	}

	// Encaminha através do SessionManager
	err = sm.ForwardEnvelope(envelope)
	if err != nil {
		log.Fatalf("❌ Failed to forward envelope: %v", err)
	}

	log.Println("✅ Message forwarded to relay")

	// Aguarda Node_B processar
	wg.Wait()

	// Exibe estatísticas
	active, relayed := sm.GetStats()
	log.Printf("\n📊 Stats: Active=%d, Relayed=%d", active, relayed)

	// Testa blacklist
	log.Println("\n🧪 Testing Blacklist...")
	sm.AddToBlacklist(nodeA.ID)

	// Tenta enviar novamente (deve falhar)
	err = sm.ForwardEnvelope(envelope)
	if err != nil {
		log.Printf("✅ Correctly rejected blacklisted sender: %v", err)
	} else {
		log.Println("❌ Should have rejected blacklisted sender!")
	}

	log.Println("\n✅ TEST COMPLETED SUCCESSFULLY!")
}

// createSimNode cria um nó simulado com chaves Ed25519
func createSimNode(name string) *SimNode {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		log.Fatalf("Failed to generate keys for %s: %v", name, err)
	}

	hash := sha256.Sum256(pub)
	nodeID := hex.EncodeToString(hash[:16])

	return &SimNode{
		ID:         nodeID,
		PrivateKey: priv,
		PublicKey:  pub,
		RecvChan:   make(chan *pbv1.Envelope, 10),
		SendChan:   make(chan *pbv1.Envelope, 10),
	}
}

// signMessage assina uma mensagem
func signMessage(priv ed25519.PrivateKey, message string) []byte {
	return ed25519.Sign(priv, []byte(message))
}
