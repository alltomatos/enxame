package main

import (
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/goautomatik/relay/internal/client"
	"github.com/goautomatik/relay/internal/config"
	"github.com/goautomatik/relay/internal/crypto"
)

func main() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)
	log.Println("🚀 Starting Enxame Relay Server...")

	// Carrega configuração
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	log.Printf("Core Address: %s", cfg.CoreAddress)
	log.Printf("Relay IP: %s", cfg.RelayIP)
	log.Printf("Region: %s", cfg.Region)

	// Carrega ou gera par de chaves
	keyPair, err := crypto.LoadOrGenerateKeyPair(cfg.PrivateKeyPath)
	if err != nil {
		log.Fatalf("Failed to load/generate keys: %v", err)
	}

	log.Printf("Node ID: %s", keyPair.NodeID)
	log.Printf("Key loaded from: %s", cfg.PrivateKeyPath)

	// Cria cliente do Core Server
	coreClient := client.NewCoreClient(cfg, keyPair)

	// Conecta ao Core Server
	if err := coreClient.Connect(); err != nil {
		log.Fatalf("Failed to connect to Core Server: %v", err)
	}
	defer coreClient.Close()

	// Registra o relay
	if err := coreClient.Register(); err != nil {
		log.Fatalf("Failed to register: %v", err)
	}

	// Inicia loop de heartbeat em goroutine
	go coreClient.StartHeartbeatLoop()

	// Inicia escuta de eventos em goroutine
	go coreClient.SubscribeToEvents()

	log.Println("✅ Relay is running and registered with Core Server")
	log.Println("Press Ctrl+C to stop")

	// Aguarda sinal de shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan

	log.Println("Shutting down...")
}
