package server

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/goautomatik/core-server/internal/crypto"
	"github.com/goautomatik/core-server/internal/domain"
	"github.com/goautomatik/core-server/internal/repository/postgres"
	"github.com/goautomatik/core-server/internal/service"
	pbv1 "github.com/goautomatik/core-server/pkg/pb/v1"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type AdminServer struct {
	pbv1.UnimplementedAdminServiceServer
	nodeService *service.NodeService
	modService  *service.ModerationService
	userRepo    *postgres.UserRepository
	nodeRepo    *postgres.NodeRepository
	masterKey   string
}

func NewAdminServer(
	ns *service.NodeService,
	ms *service.ModerationService,
	ur *postgres.UserRepository,
	nr *postgres.NodeRepository,
) *AdminServer {
	return &AdminServer{
		nodeService: ns,
		modService:  ms,
		userRepo:    ur,
		nodeRepo:    nr,
		masterKey:   os.Getenv("ADMIN_MASTER_KEY"),
	}
}

func (s *AdminServer) Register(server *grpc.Server) {
	pbv1.RegisterAdminServiceServer(server, s)
}

func (s *AdminServer) GetNetworkStats(ctx context.Context, req *pbv1.GetNetworkStatsRequest) (*pbv1.GetNetworkStatsResponse, error) {
	nodeStats, err := s.nodeService.GetNodeStats(ctx)
	if err != nil {
		return nil, err
	}

	totalUsers, _ := s.userRepo.GetTotalUsers(ctx)
	totalNodes, _ := s.nodeRepo.GetTotalNodesCount(ctx)

	onlineNodes := 0
	typeStats := make(map[string]int32)
	for t, count := range nodeStats {
		onlineNodes += count
		typeStats[t.String()] = int32(count)
	}

	return &pbv1.GetNetworkStatsResponse{
		TotalNodes:  int32(totalNodes),
		OnlineNodes: int32(onlineNodes),
		TotalUsers:  int32(totalUsers),
		NodesByType: typeStats,
	}, nil
}

func (s *AdminServer) ListNodes(ctx context.Context, req *pbv1.ListNodesRequest) (*pbv1.ListNodesResponse, error) {
	nodes, total, err := s.nodeRepo.ListNodes(ctx, int(req.FilterType), req.OnlineOnly, int(req.Limit), int(req.Offset))
	if err != nil {
		return nil, err
	}

	var pbNodes []*pbv1.NodeInfo
	for _, n := range nodes {
		pbNodes = append(pbNodes, &pbv1.NodeInfo{
			Identity: &pbv1.NodeIdentity{
				NodeId:    n.Identity.NodeID,
				PublicKey: n.Identity.PublicKey,
			},
			Type:         pbv1.NodeType(n.Type),
			Status:       pbv1.NodeStatus(n.Status),
			Endpoints:    n.Endpoints,
			Version:      n.Version,
			Capabilities: n.Capabilities,
			Region:       n.Region,
			RegisteredAt: timestamppb.New(n.RegisteredAt),
			LastSeen:     timestamppb.New(n.LastSeen),
		})
	}

	return &pbv1.ListNodesResponse{
		Nodes:      pbNodes,
		TotalCount: int32(total),
	}, nil
}

func (s *AdminServer) ListUsers(ctx context.Context, req *pbv1.ListUsersRequest) (*pbv1.ListUsersResponse, error) {
	users, total, err := s.userRepo.ListUsers(ctx, int(req.Limit), int(req.Offset))
	if err != nil {
		return nil, err
	}

	var pbUsers []*pbv1.UserSummary
	for _, u := range users {
		pbUsers = append(pbUsers, &pbv1.UserSummary{
			Id:       u.ID,
			Email:    u.Email,
			Nickname: u.Nickname,
			Role:     u.Role,
			IsOnline: true,
		})
	}

	return &pbv1.ListUsersResponse{
		Users:      pbUsers,
		TotalCount: int32(total),
	}, nil
}

func (s *AdminServer) UpdateUserRole(ctx context.Context, req *pbv1.UpdateUserRoleRequest) (*pbv1.UpdateUserRoleResponse, error) {
	err := s.userRepo.UpdateUserRole(ctx, req.UserId, req.NewRole)
	if err != nil {
		return &pbv1.UpdateUserRoleResponse{Success: false, Message: err.Error()}, nil
	}
	return &pbv1.UpdateUserRoleResponse{Success: true, Message: "Role updated successfully"}, nil
}

func (s *AdminServer) BanNode(ctx context.Context, req *pbv1.BanNodeRequest) (*pbv1.BanNodeResponse, error) {
	ban := &domain.NodeBan{
		BannedNodeID:    req.NodeId,
		Reason:          req.Reason,
		DurationSeconds: req.DurationSeconds,
		BannedAt:        time.Now(),
	}
	if req.DurationSeconds > 0 {
		exp := time.Now().Add(time.Duration(req.DurationSeconds) * time.Second)
		ban.ExpiresAt = &exp
	}

	if err := s.nodeRepo.BanNode(ctx, ban); err != nil {
		return &pbv1.BanNodeResponse{Success: false, Message: err.Error()}, nil
	}

	return &pbv1.BanNodeResponse{Success: true, Message: "Node banned successfully"}, nil
}

func (s *AdminServer) UnbanNode(ctx context.Context, req *pbv1.UnbanNodeRequest) (*pbv1.UnbanNodeResponse, error) {
	if err := s.nodeRepo.UnbanNode(ctx, req.NodeId); err != nil {
		return &pbv1.UnbanNodeResponse{Success: false, Message: err.Error()}, nil
	}
	return &pbv1.UnbanNodeResponse{Success: true, Message: "Node unbanned successfully"}, nil
}

func (s *AdminServer) SendGlobalAlert(ctx context.Context, req *pbv1.SendGlobalAlertRequest) (*pbv1.SendGlobalAlertResponse, error) {
	alert := &domain.GlobalAlert{
		AlertID:   fmt.Sprintf("alert-%d", time.Now().Unix()),
		Title:     req.Title,
		Message:   req.Message,
		Severity:  req.Severity,
		InfoURL:   req.InfoUrl,
		CreatedAt: time.Now(),
	}

	// Obtém o ID do moderador do contexto JWT se disponível
	moderatorID, _ := ctx.Value("user_id").(string)

	err := s.modService.PublishInternalAlert(ctx, alert, moderatorID, nil, time.Now())
	if err != nil {
		return &pbv1.SendGlobalAlertResponse{Success: false, Message: err.Error()}, nil
	}

	return &pbv1.SendGlobalAlertResponse{Success: true, Message: "Alert broadcasted", EventId: alert.AlertID}, nil
}

func (s *AdminServer) VerifyMasterKey(ctx context.Context, req *pbv1.VerifyMasterKeyRequest) (*pbv1.VerifyMasterKeyResponse, error) {
	if s.masterKey == "" {
		return &pbv1.VerifyMasterKeyResponse{
			Success: false,
			Message: "ADMIN_MASTER_KEY not configured on server",
		}, nil
	}

	if req.MasterKey != s.masterKey {
		return &pbv1.VerifyMasterKeyResponse{
			Success: false,
			Message: "Invalid master key",
		}, nil
	}

	userID, _ := ctx.Value("user_id").(string)
	if userID == "" {
		userID = "admin-session"
	}

	vaultToken, err := crypto.GenerateVaultToken(userID)
	if err != nil {
		return nil, err
	}

	return &pbv1.VerifyMasterKeyResponse{
		Success:    true,
		Message:    "Master key verified. Vault unlocked for 30 minutes.",
		VaultToken: vaultToken,
	}, nil
}
