package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/go-redis/redis/v8"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/goautomatik/core-server/internal/config"
	"github.com/goautomatik/core-server/internal/crypto"
	"github.com/goautomatik/core-server/internal/repository/postgres"
	redisrepo "github.com/goautomatik/core-server/internal/repository/redis"
	"github.com/goautomatik/core-server/internal/server"
	"github.com/goautomatik/core-server/internal/service"
)

func main() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)
	log.Println("Starting Core Server...")

	// Carrega configurações
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	log.Printf("Environment: %s", cfg.Environment)
	log.Printf("gRPC Port: %d", cfg.GRPCPort)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Conecta ao PostgreSQL
	pgPool, err := connectPostgres(ctx, cfg)
	if err != nil {
		log.Fatalf("Failed to connect to PostgreSQL: %v", err)
	}
	defer pgPool.Close()
	log.Println("Connected to PostgreSQL")

	// Conecta ao Redis
	redisClient, err := connectRedis(ctx, cfg)
	if err != nil {
		log.Fatalf("Failed to connect to Redis: %v", err)
	}
	defer redisClient.Close()
	log.Println("Connected to Redis")

	// Inicializa repositórios
	pgRepo := postgres.NewNodeRepository(pgPool)
	if err := pgRepo.CreateTables(ctx); err != nil {
		log.Fatalf("Failed to create database tables: %v", err)
	}
	log.Println("Database tables initialized")

	redisManager := redisrepo.NewNodeManager(redisClient, cfg.HeartbeatTTL)

	// Inicializa verificador de assinaturas
	verifier, err := crypto.NewVerifier(cfg.MasterPublicKeys)
	if err != nil {
		log.Fatalf("Failed to initialize signature verifier: %v", err)
	}
	log.Printf("Initialized with %d master keys", len(cfg.MasterPublicKeys))

	// Inicializa serviços
	nodeService := service.NewNodeService(
		redisManager,
		pgRepo,
		cfg.HeartbeatTTL,
		cfg.MaxRelaysPerResponse,
	)

	modService := service.NewModerationService(
		redisManager,
		pgRepo,
		verifier,
	)

	// Inicializa interceptor de autenticação
	authInterceptor := server.NewAuthInterceptor(verifier)

	// Inicializa verificador de saúde (K8s Health Check)
	healthChecker := server.NewHealthChecker(redisManager, pgRepo)

	// Inicializa e inicia o servidor gRPC
	grpcServer := server.NewGRPCServer(cfg, nodeService, modService, authInterceptor, healthChecker)

	// Canal para capturar sinais de shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Inicia o servidor em uma goroutine
	go func() {
		port := strconv.Itoa(cfg.GRPCPort)
		if err := grpcServer.Start(port); err != nil {
			log.Fatalf("Failed to start gRPC server: %v", err)
		}
	}()

	log.Printf("Core Server is running on port %d", cfg.GRPCPort)

	// Aguarda sinal de shutdown
	sig := <-sigChan
	log.Printf("Received signal %v, shutting down...", sig)

	// Graceful shutdown
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()

	grpcServer.Stop()
	cancel()

	// Aguarda o contexto de shutdown
	<-shutdownCtx.Done()
	log.Println("Core Server stopped")
}

func connectPostgres(ctx context.Context, cfg *config.Config) (*pgxpool.Pool, error) {
	dsn := fmt.Sprintf(
		"postgres://%s:%s@%s:%d/%s?sslmode=%s&pool_max_conns=%d",
		cfg.PostgresUser,
		cfg.PostgresPassword,
		cfg.PostgresHost,
		cfg.PostgresPort,
		cfg.PostgresDB,
		cfg.PostgresSSL,
		cfg.PostgresPoolSize,
	)

	poolConfig, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to parse postgres config: %w", err)
	}

	// Configurações otimizadas do pool
	poolConfig.MaxConns = int32(cfg.PostgresPoolSize)
	poolConfig.MinConns = 5
	poolConfig.MaxConnLifetime = 30 * time.Minute
	poolConfig.MaxConnIdleTime = 5 * time.Minute
	poolConfig.HealthCheckPeriod = 1 * time.Minute

	pool, err := pgxpool.NewWithConfig(ctx, poolConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create postgres pool: %w", err)
	}

	// Testa a conexão
	if err := pool.Ping(ctx); err != nil {
		return nil, fmt.Errorf("failed to ping postgres: %w", err)
	}

	return pool, nil
}

func connectRedis(ctx context.Context, cfg *config.Config) (*redis.Client, error) {
	client := redis.NewClient(&redis.Options{
		Addr:         fmt.Sprintf("%s:%d", cfg.RedisHost, cfg.RedisPort),
		Password:     cfg.RedisPassword,
		DB:           cfg.RedisDB,
		PoolSize:     cfg.RedisPoolSize,
		MinIdleConns: 10,
		MaxRetries:   3,
		DialTimeout:  5 * time.Second,
		ReadTimeout:  3 * time.Second,
		WriteTimeout: 3 * time.Second,
		PoolTimeout:  4 * time.Second,
	})

	// Testa a conexão
	if err := client.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("failed to ping redis: %w", err)
	}

	return client, nil
}
