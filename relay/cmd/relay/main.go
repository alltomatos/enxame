package main

import (
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/keepalive"

	"github.com/goautomatik/relay/internal/client"
	"github.com/goautomatik/relay/internal/config"
	"github.com/goautomatik/relay/internal/crypto"

	// Importa o pacote pkg/relay que agora contém a lógica do servidor
	pkgrelay "github.com/goautomatik/core-server/pkg/relay"
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
	log.Printf("Relay Port: %d", cfg.RelayPort)
	log.Printf("Region: %s", cfg.Region)

	// Carrega ou gera par de chaves
	keyPair, err := crypto.LoadOrGenerateKeyPair(cfg.PrivateKeyPath)
	if err != nil {
		log.Fatalf("Failed to load/generate keys: %v", err)
	}

	log.Printf("Node ID: %s", keyPair.NodeID)

	// Cria Session Manager
	sessionManager := pkgrelay.NewSessionManager()

	// Cria Relay Server
	relayServer := pkgrelay.NewRelayServer(cfg.RelayPort, sessionManager)

	// Cria listener TCP
	lis, err := net.Listen("tcp", fmt.Sprintf(":%d", cfg.RelayPort))
	if err != nil {
		log.Fatalf("Failed to listen: %v", err)
	}

	// Configura servidor gRPC
	opts := []grpc.ServerOption{
		grpc.MaxConcurrentStreams(10000),
		grpc.KeepaliveParams(keepalive.ServerParameters{
			Time:    1 * time.Minute,
			Timeout: 20 * time.Second,
		}),
	}
	grpcServer := grpc.NewServer(opts...)

	// Registra serviço usando handler manual
	pkgrelay.RegisterRelayServiceServer(grpcServer, relayServer)

	// Inicia servidor gRPC em goroutine
	go func() {
		log.Printf("📡 gRPC Server listening on :%d", cfg.RelayPort)
		if err := grpcServer.Serve(lis); err != nil {
			log.Fatalf("Failed to serve: %v", err)
		}
	}()

	// Cria cliente do Core Server
	coreClient := client.NewCoreClient(cfg, keyPair)

	// Conecta ao Core Server
	if err := coreClient.Connect(); err != nil {
		log.Printf("⚠️ Core connect failed (will retry): %v", err)
		// Continua rodando mesmo sem Core inicialmente
	} else {
		defer coreClient.Close()

		// Registra o relay
		if err := coreClient.Register(); err != nil {
			log.Printf("⚠️ Register failed: %v", err)
		} else {
			// Inicia loop de heartbeat em goroutine
			go coreClient.StartHeartbeatLoop()

			// Inicia escuta de eventos em goroutine
			go coreClient.SubscribeToEvents()
		}
	}

	log.Println("✅ Relay is ready!")
	log.Println("Press Ctrl+C to stop")

	// Aguarda sinal de shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan

	log.Println("Shutting down...")
	grpcServer.GracefulStop()
}
