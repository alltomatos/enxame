package server

import (
	"context"
	"log"
	"os"

	pbv1 "github.com/goautomatik/core-server/pkg/pb/v1"
	"google.golang.org/grpc"
)

type UpdateServer struct {
	pbv1.UnimplementedUpdateServiceServer
	latestVersion string
	updateURL     string
}

func NewUpdateServer() *UpdateServer {
	return &UpdateServer{
		latestVersion: os.Getenv("LATEST_VERSION"),
		updateURL:     os.Getenv("UPDATE_URL"),
	}
}

func (s *UpdateServer) Register(server *grpc.Server) {
	pbv1.RegisterUpdateServiceServer(server, s)
}

func (s *UpdateServer) CheckForUpdate(ctx context.Context, req *pbv1.CheckForUpdateRequest) (*pbv1.CheckForUpdateResponse, error) {
	log.Printf("[Update] Check from version %s (platform: %s)", req.CurrentVersion, req.Platform)

	if s.latestVersion == "" || s.updateURL == "" {
		return &pbv1.CheckForUpdateResponse{
			Available: false,
		}, nil
	}

	// Lógica simples de comparação: se for diferente, avisar (para teste)
	// Em produção usaríamos semver
	if req.CurrentVersion != s.latestVersion {
		return &pbv1.CheckForUpdateResponse{
			Available: true,
			Version:   s.latestVersion,
			Url:       s.updateURL,
			Critical:  false, // TODO: ler de env se necessário
		}, nil
	}

	return &pbv1.CheckForUpdateResponse{
		Available: false,
	}, nil
}
