package server

import (
	"context"
	"log"
	"sync"

	"github.com/goautomatik/core-server/internal/repository/postgres"
	"github.com/goautomatik/core-server/internal/repository/redis"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
)

// HealthChecker implementa verificações de saúde para Kubernetes
type HealthChecker struct {
	redisManager *redis.NodeManager
	pgRepo       *postgres.NodeRepository
	healthServer *health.Server

	mu      sync.RWMutex
	healthy bool
}

// NewHealthChecker cria um novo verificador de saúde
func NewHealthChecker(redisManager *redis.NodeManager, pgRepo *postgres.NodeRepository) *HealthChecker {
	hc := &HealthChecker{
		redisManager: redisManager,
		pgRepo:       pgRepo,
		healthServer: health.NewServer(),
		healthy:      true,
	}

	// Define status inicial como serving
	hc.healthServer.SetServingStatus("", healthpb.HealthCheckResponse_SERVING)
	hc.healthServer.SetServingStatus("network.v1.NetworkService", healthpb.HealthCheckResponse_SERVING)

	return hc
}

// GetHealthServer retorna o servidor de health check para registrar no gRPC
func (hc *HealthChecker) GetHealthServer() *health.Server {
	return hc.healthServer
}

// Check verifica a saúde das dependências (Redis e PostgreSQL)
func (hc *HealthChecker) Check(ctx context.Context) error {
	// Verifica Redis
	if err := hc.redisManager.Ping(ctx); err != nil {
		log.Printf("[Health] Redis check failed: %v", err)
		hc.setUnhealthy()
		return err
	}

	// Verifica PostgreSQL
	if err := hc.pgRepo.Ping(ctx); err != nil {
		log.Printf("[Health] PostgreSQL check failed: %v", err)
		hc.setUnhealthy()
		return err
	}

	hc.setHealthy()
	return nil
}

// setHealthy marca o serviço como saudável
func (hc *HealthChecker) setHealthy() {
	hc.mu.Lock()
	defer hc.mu.Unlock()

	if !hc.healthy {
		log.Println("[Health] Service recovered, marking as SERVING")
		hc.healthy = true
	}

	hc.healthServer.SetServingStatus("", healthpb.HealthCheckResponse_SERVING)
	hc.healthServer.SetServingStatus("network.v1.NetworkService", healthpb.HealthCheckResponse_SERVING)
}

// setUnhealthy marca o serviço como não saudável
func (hc *HealthChecker) setUnhealthy() {
	hc.mu.Lock()
	defer hc.mu.Unlock()

	if hc.healthy {
		log.Println("[Health] Service unhealthy, marking as NOT_SERVING")
		hc.healthy = false
	}

	hc.healthServer.SetServingStatus("", healthpb.HealthCheckResponse_NOT_SERVING)
	hc.healthServer.SetServingStatus("network.v1.NetworkService", healthpb.HealthCheckResponse_NOT_SERVING)
}

// IsHealthy retorna o estado de saúde atual
func (hc *HealthChecker) IsHealthy() bool {
	hc.mu.RLock()
	defer hc.mu.RUnlock()
	return hc.healthy
}
