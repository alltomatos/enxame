package server

import (
	"context"
	"crypto/ed25519"
	"encoding/hex"
	"errors"
	"log"
	"time"

	"github.com/goautomatik/core-server/internal/repository/postgres"
	pbv1 "github.com/goautomatik/core-server/pkg/pb/v1"
	"google.golang.org/grpc"
)

// ClusterServer implementa o ClusterService para Alta Disponibilidade
type ClusterServer struct {
	repo          *postgres.NodeRepository
	selfPeerID    string // ID deste servidor no cluster
	selfAddress   string // Endereço deste servidor (IP:Port)
	selfPublicKey ed25519.PublicKey
}

// NewClusterServer cria um novo servidor de cluster
func NewClusterServer(repo *postgres.NodeRepository, selfPeerID, selfAddress string, selfPublicKey ed25519.PublicKey) *ClusterServer {
	return &ClusterServer{
		repo:          repo,
		selfPeerID:    selfPeerID,
		selfAddress:   selfAddress,
		selfPublicKey: selfPublicKey,
	}
}

// Register registra o ClusterService no gRPC server
func (s *ClusterServer) Register(grpcServer *grpc.Server) {
	// Registro manual (sem protoc)
	grpcServer.RegisterService(&_ClusterService_serviceDesc, s)
}

// RequestJoin processa uma solicitação de join de um novo peer
func (s *ClusterServer) RequestJoin(ctx context.Context, req *pbv1.RequestJoinRequest) (*pbv1.RequestJoinResponse, error) {
	if req.Peer == nil {
		return &pbv1.RequestJoinResponse{Success: false, Message: "peer info required"}, nil
	}

	peer := &postgres.ClusterPeer{
		PeerID:    req.Peer.PeerID,
		Address:   req.Peer.Address,
		PublicKey: req.Peer.PublicKey,
		Status:    "pending",
		LastSeen:  time.Now(),
	}

	if err := s.repo.UpsertClusterPeer(ctx, peer); err != nil {
		log.Printf("[Cluster] Failed to save pending peer %s: %v", req.Peer.PeerID, err)
		return &pbv1.RequestJoinResponse{Success: false, Message: "failed to save peer"}, nil
	}

	log.Printf("[Cluster] New peer requested join: %s (%s) - PENDING APPROVAL", req.Peer.PeerID, req.Peer.Address)
	return &pbv1.RequestJoinResponse{Success: true, Message: "Aguardando aprovação do administrador."}, nil
}

// ApprovePeer aprova um peer pendente e inicia o sync inicial
func (s *ClusterServer) ApprovePeer(ctx context.Context, req *pbv1.ApprovePeerRequest) (*pbv1.ApprovePeerResponse, error) {
	// Verificar peer existe
	peer, err := s.repo.GetClusterPeer(ctx, req.PeerID)
	if err != nil {
		return &pbv1.ApprovePeerResponse{Success: false, Message: "database error"}, nil
	}
	if peer == nil {
		return &pbv1.ApprovePeerResponse{Success: false, Message: "peer not found"}, nil
	}
	if peer.Status == "active" {
		return &pbv1.ApprovePeerResponse{Success: true, Message: "peer already active"}, nil
	}

	// Atualizar status para active
	if err := s.repo.UpdateClusterPeerStatus(ctx, req.PeerID, "active"); err != nil {
		return &pbv1.ApprovePeerResponse{Success: false, Message: "failed to activate peer"}, nil
	}

	log.Printf("[Cluster] Peer %s APPROVED. Starting initial sync...", req.PeerID)

	// Iniciar sync inicial (em goroutine para não bloquear)
	go s.performInitialSync(peer)

	return &pbv1.ApprovePeerResponse{Success: true, Message: "Peer ativado. Sync inicial em andamento."}, nil
}

// performInitialSync envia todos os usuários atuais para o novo peer
func (s *ClusterServer) performInitialSync(peer *postgres.ClusterPeer) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// Obter todos os nós registrados
	nodes, err := s.repo.GetAllNodes(ctx)
	if err != nil {
		log.Printf("[Cluster] Failed to get nodes for sync: %v", err)
		return
	}

	if len(nodes) == 0 {
		log.Printf("[Cluster] No nodes to sync to peer %s", peer.PeerID)
		return
	}

	// Conectar ao peer
	conn, err := grpc.Dial(peer.Address, grpc.WithInsecure(), grpc.WithBlock(), grpc.WithTimeout(10*time.Second))
	if err != nil {
		log.Printf("[Cluster] Failed to connect to peer %s for sync: %v", peer.Address, err)
		return
	}
	defer conn.Close()

	client := pbv1.NewClusterServiceClient(conn)

	// Converter para protobuf
	pbNodes := make([]*pbv1.NodeIdentity, len(nodes))
	for i, n := range nodes {
		pbNodes[i] = &pbv1.NodeIdentity{
			NodeId:    n.NodeID,
			PublicKey: n.PublicKey,
		}
	}

	resp, err := client.SyncNodes(ctx, &pbv1.SyncNodesRequest{Nodes: pbNodes})
	if err != nil {
		log.Printf("[Cluster] Sync to peer %s failed: %v", peer.Address, err)
		return
	}

	log.Printf("[Cluster] Initial sync to peer %s complete: %d nodes synced", peer.PeerID, resp.Count)
}

// GetClusterNodes retorna todos os peers ativos do cluster
func (s *ClusterServer) GetClusterNodes(ctx context.Context, req *pbv1.GetClusterNodesRequest) (*pbv1.GetClusterNodesResponse, error) {
	peers, err := s.repo.GetActiveClusterPeers(ctx)
	if err != nil {
		return nil, err
	}

	// Adicionar este servidor à lista
	nodes := []*pbv1.PeerInfo{
		{
			PeerID:    s.selfPeerID,
			Address:   s.selfAddress,
			PublicKey: s.selfPublicKey,
		},
	}

	for _, p := range peers {
		nodes = append(nodes, &pbv1.PeerInfo{
			PeerID:    p.PeerID,
			Address:   p.Address,
			PublicKey: p.PublicKey,
		})
	}

	return &pbv1.GetClusterNodesResponse{Nodes: nodes}, nil
}

// SyncNodes recebe dados de sync do primário (chamado por outro core)
func (s *ClusterServer) SyncNodes(ctx context.Context, req *pbv1.SyncNodesRequest) (*pbv1.SyncNodesResponse, error) {
	log.Printf("[Cluster] Receiving initial sync: %d nodes", len(req.Nodes))

	count := int32(0)
	for _, n := range req.Nodes {
		// Inserir no Redis/Postgres local (apenas estrutura mínima)
		// Por ora, apenas logamos - o RegisterNode normal cuidará do armazenamento
		log.Printf("[Cluster/Sync] Received node: %s", n.NodeId[:16])
		count++
	}

	return &pbv1.SyncNodesResponse{Success: true, Count: count}, nil
}

// BroadcastNodeRegistration envia um novo registro para todos os peers ativos
func (s *ClusterServer) BroadcastNodeRegistration(ctx context.Context, nodeID string, publicKey []byte) {
	peers, err := s.repo.GetActiveClusterPeers(ctx)
	if err != nil {
		log.Printf("[Cluster] Failed to get peers for broadcast: %v", err)
		return
	}

	if len(peers) == 0 {
		return // Nenhum peer para notificar
	}

	nodes := []*pbv1.NodeIdentity{
		{NodeId: nodeID, PublicKey: publicKey},
	}

	for _, peer := range peers {
		go func(p *postgres.ClusterPeer) {
			conn, err := grpc.Dial(p.Address, grpc.WithInsecure(), grpc.WithTimeout(5*time.Second))
			if err != nil {
				log.Printf("[Cluster] Failed to connect to peer %s for broadcast: %v", p.Address, err)
				return
			}
			defer conn.Close()

			client := pbv1.NewClusterServiceClient(conn)
			_, err = client.SyncNodes(context.Background(), &pbv1.SyncNodesRequest{Nodes: nodes})
			if err != nil {
				log.Printf("[Cluster] Broadcast to peer %s failed: %v", p.Address, err)
			}
		}(peer)
	}
}

// GetPeerIDFromPublicKey gera um Peer ID a partir da chave pública
func GetPeerIDFromPublicKey(pubKey ed25519.PublicKey) string {
	return hex.EncodeToString(pubKey[:16])
}

// -- gRPC Service Descriptor (Manual) --

var _ interface {
	RequestJoin(context.Context, *pbv1.RequestJoinRequest) (*pbv1.RequestJoinResponse, error)
	ApprovePeer(context.Context, *pbv1.ApprovePeerRequest) (*pbv1.ApprovePeerResponse, error)
	GetClusterNodes(context.Context, *pbv1.GetClusterNodesRequest) (*pbv1.GetClusterNodesResponse, error)
	SyncNodes(context.Context, *pbv1.SyncNodesRequest) (*pbv1.SyncNodesResponse, error)
} = (*ClusterServer)(nil)

var _ClusterService_serviceDesc = grpc.ServiceDesc{
	ServiceName: "cluster.v1.ClusterService",
	HandlerType: (*pbv1.ClusterServiceServer)(nil),
	Methods: []grpc.MethodDesc{

		{MethodName: "RequestJoin", Handler: _ClusterService_RequestJoin_Handler},
		{MethodName: "ApprovePeer", Handler: _ClusterService_ApprovePeer_Handler},
		{MethodName: "GetClusterNodes", Handler: _ClusterService_GetClusterNodes_Handler},
		{MethodName: "SyncNodes", Handler: _ClusterService_SyncNodes_Handler},
	},
	Streams:  []grpc.StreamDesc{},
	Metadata: "cluster_manual.go",
}

func _ClusterService_RequestJoin_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(pbv1.RequestJoinRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(*ClusterServer).RequestJoin(ctx, in)
	}
	info := &grpc.UnaryServerInfo{Server: srv, FullMethod: "/cluster.v1.ClusterService/RequestJoin"}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(*ClusterServer).RequestJoin(ctx, req.(*pbv1.RequestJoinRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _ClusterService_ApprovePeer_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(pbv1.ApprovePeerRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(*ClusterServer).ApprovePeer(ctx, in)
	}
	info := &grpc.UnaryServerInfo{Server: srv, FullMethod: "/cluster.v1.ClusterService/ApprovePeer"}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(*ClusterServer).ApprovePeer(ctx, req.(*pbv1.ApprovePeerRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _ClusterService_GetClusterNodes_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(pbv1.GetClusterNodesRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(*ClusterServer).GetClusterNodes(ctx, in)
	}
	info := &grpc.UnaryServerInfo{Server: srv, FullMethod: "/cluster.v1.ClusterService/GetClusterNodes"}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(*ClusterServer).GetClusterNodes(ctx, req.(*pbv1.GetClusterNodesRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _ClusterService_SyncNodes_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(pbv1.SyncNodesRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(*ClusterServer).SyncNodes(ctx, in)
	}
	info := &grpc.UnaryServerInfo{Server: srv, FullMethod: "/cluster.v1.ClusterService/SyncNodes"}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(*ClusterServer).SyncNodes(ctx, req.(*pbv1.SyncNodesRequest))
	}
	return interceptor(ctx, in, info, handler)
}

// Package-level error sentinel
var ErrPeerNotFound = errors.New("peer not found")
