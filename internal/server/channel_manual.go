package server

import (
	"context"
	"crypto/ed25519"
	"errors"
	"fmt"
	"log"
	"sync"
	"time"

	"google.golang.org/grpc"

	"github.com/goautomatik/core-server/internal/service"
	pbv1 "github.com/goautomatik/core-server/pkg/pb/v1"
)

// ChannelService define as operações manuais de Canal
type ChannelService interface {
	CreateChannel(ctx context.Context, req *pbv1.CreateChannelRequest) (*pbv1.CreateChannelResponse, error)
	// JoinChannel... (Legacy/Relay)
	GetChannelInfo(ctx context.Context, req *pbv1.GetChannelInfoRequest) (*pbv1.GetChannelInfoResponse, error)
	PromoteUser(ctx context.Context, req *pbv1.PromoteRequest) (*pbv1.PromoteResponse, error)
	DemoteUser(ctx context.Context, req *pbv1.DemoteRequest) (*pbv1.DemoteResponse, error)
	KickUser(ctx context.Context, req *pbv1.KickRequest) (*pbv1.KickResponse, error)
	UpdateChannelAvatar(ctx context.Context, req *pbv1.UpdateChannelAvatarRequest) (*pbv1.UpdateChannelAvatarResponse, error)
}

// -- Implementação do Servidor --

type ChannelServerImpl struct {
	channels    sync.Map // map[string]*ChannelData
	nodeService *service.NodeService
}

type ChannelData struct {
	ID        string
	Name      string
	OwnerID   string
	Members   map[string]bool   // NodeID -> Exists
	Roles     map[string]string // NodeID -> Role (ADMIN, MODERATOR)
	Avatar    string            // Base64
	CreatedAt time.Time
	mu        sync.RWMutex
}

func NewChannelServer(nodeService *service.NodeService) *ChannelServerImpl {
	s := &ChannelServerImpl{
		channels:    sync.Map{},
		nodeService: nodeService,
	}
	s.bootstrap()
	return s
}

func (s *ChannelServerImpl) bootstrap() {
	// Garante que o canal #Inicio exista (renomeando o antigo #geral se necessário)
	// Como o servidor armazena em memória (sync.Map), apenas criamos o #Inicio no boot.

	s.channels.Store("#Inicio", &ChannelData{
		ID:        "#Inicio",
		Name:      "Inicio",
		OwnerID:   "SYSTEM",
		Members:   make(map[string]bool),
		Roles:     make(map[string]string),
		CreatedAt: time.Now(),
	})
	log.Println("Channel bootstrap completed: #Inicio created")
}

func (s *ChannelServerImpl) CreateChannel(ctx context.Context, req *pbv1.CreateChannelRequest) (*pbv1.CreateChannelResponse, error) {
	if req.Name == "" || req.OwnerNodeId == "" {
		return &pbv1.CreateChannelResponse{Success: false, Message: "Name and OwnerID required"}, nil
	}

	// Gera ID simples (hash do nome idealmente, aqui simplificado)
	slug := fmt.Sprintf("#%s", req.Name)

	if _, exists := s.channels.Load(slug); exists {
		return &pbv1.CreateChannelResponse{Success: false, Message: "Channel already exists"}, nil
	}

	ch := &ChannelData{
		ID:        slug,
		Name:      req.Name,
		OwnerID:   req.OwnerNodeId,
		Members:   make(map[string]bool),
		Roles:     make(map[string]string),
		Avatar:    req.Avatar,
		CreatedAt: time.Now(),
	}
	ch.Members[req.OwnerNodeId] = true
	ch.Roles[req.OwnerNodeId] = pbv1.Role_ADMIN

	s.channels.Store(slug, ch)

	return &pbv1.CreateChannelResponse{
		Success:   true,
		Message:   "Channel created",
		ChannelId: slug,
	}, nil
}

func (s *ChannelServerImpl) GetChannelInfo(ctx context.Context, req *pbv1.GetChannelInfoRequest) (*pbv1.GetChannelInfoResponse, error) {
	val, ok := s.channels.Load(req.ChannelId)
	if !ok {
		return &pbv1.GetChannelInfoResponse{}, fmt.Errorf("channel not found")
	}
	ch := val.(*ChannelData)
	return &pbv1.GetChannelInfoResponse{
		ChannelId: ch.ID,
		Name:      ch.Name,
		OwnerId:   ch.OwnerID,
		Avatar:    ch.Avatar,
	}, nil
}

func (s *ChannelServerImpl) PromoteUser(ctx context.Context, req *pbv1.PromoteRequest) (*pbv1.PromoteResponse, error) {
	// Verify Signature
	if err := s.verifyGovernanceSignature(ctx, req.RequesterId, req.Signature, req.TargetNodeId, req.Role, req.Scope, req.ChannelId, req.Timestamp); err != nil {
		return &pbv1.PromoteResponse{Success: false, Message: "Signature verification failed: " + err.Error()}, nil
	}

	// 1. Verify Scope (MVP: Channel Only for now, Global logic later)
	if req.Scope != pbv1.Scope_CHANNEL {
		return &pbv1.PromoteResponse{Success: false, Message: "Only CHANNEL scope supported in MVP"}, nil
	}

	val, ok := s.channels.Load(req.ChannelId)
	if !ok {
		return &pbv1.PromoteResponse{Success: false, Message: "Channel not found"}, nil
	}
	ch := val.(*ChannelData)

	ch.mu.Lock()
	defer ch.mu.Unlock()

	// 2. Auth Check (Must be Owner or Admin)
	requesterRole := ch.Roles[req.RequesterId]
	if requesterRole != pbv1.Role_ADMIN {
		return &pbv1.PromoteResponse{Success: false, Message: "Permission denied. Only ADMIN/OWNER can promote."}, nil
	}

	// 3. Apply Role
	ch.Roles[req.TargetNodeId] = req.Role

	// Audit Log
	s.nodeService.LogGovernanceAction(ctx, req.RequesterId, "PROMOTE", req.TargetNodeId, req.ChannelId, req.Signature)

	return &pbv1.PromoteResponse{Success: true, Message: "User promoted"}, nil
}

func (s *ChannelServerImpl) DemoteUser(ctx context.Context, req *pbv1.DemoteRequest) (*pbv1.DemoteResponse, error) {
	// Verify Signature
	if err := s.verifyGovernanceSignature(ctx, req.RequesterId, req.Signature, req.TargetNodeId, "", req.Scope, req.ChannelId, req.Timestamp); err != nil {
		return &pbv1.DemoteResponse{Success: false, Message: "Signature verification failed: " + err.Error()}, nil
	}

	if req.Scope != pbv1.Scope_CHANNEL {
		return &pbv1.DemoteResponse{Success: false, Message: "Only CHANNEL scope supported"}, nil
	}

	val, ok := s.channels.Load(req.ChannelId)
	if !ok {
		return &pbv1.DemoteResponse{Success: false, Message: "Channel not found"}, nil
	}
	ch := val.(*ChannelData)

	ch.mu.Lock()
	defer ch.mu.Unlock()

	if ch.Roles[req.RequesterId] != pbv1.Role_ADMIN {
		return &pbv1.DemoteResponse{Success: false, Message: "Permission denied"}, nil
	}

	// Remove Role (Downgrade to MEMBER)
	delete(ch.Roles, req.TargetNodeId)

	// Audit Log
	s.nodeService.LogGovernanceAction(ctx, req.RequesterId, "DEMOTE", req.TargetNodeId, req.ChannelId, req.Signature)

	return &pbv1.DemoteResponse{Success: true, Message: "User demoted"}, nil
}

func (s *ChannelServerImpl) KickUser(ctx context.Context, req *pbv1.KickRequest) (*pbv1.KickResponse, error) {
	// Verify Signature
	if err := s.verifyGovernanceSignature(ctx, req.RequesterId, req.Signature, req.TargetNodeId, "", req.Scope, req.ChannelId, req.Timestamp); err != nil {
		return &pbv1.KickResponse{Success: false, Message: "Signature verification failed: " + err.Error()}, nil
	}

	if req.Scope != pbv1.Scope_CHANNEL {
		return &pbv1.KickResponse{Success: false, Message: "Only CHANNEL scope supported"}, nil
	}

	val, ok := s.channels.Load(req.ChannelId)
	if !ok {
		return &pbv1.KickResponse{Success: false, Message: "Channel not found"}, nil
	}
	ch := val.(*ChannelData)

	ch.mu.Lock()
	defer ch.mu.Unlock()

	// Auth: Requester must be MODERATOR or ADMIN (Owner)
	role := ch.Roles[req.RequesterId]
	if role != pbv1.Role_ADMIN && role != pbv1.Role_MODERATOR {
		return &pbv1.KickResponse{Success: false, Message: "Permission denied"}, nil
	}

	// Logic: Remove from Members
	delete(ch.Members, req.TargetNodeId)
	delete(ch.Roles, req.TargetNodeId) // Also remove roles if any

	// audit log
	s.nodeService.LogGovernanceAction(ctx, req.RequesterId, "KICK", req.TargetNodeId, req.ChannelId, req.Signature)

	// Note: Authentication for Kick should ideally notify Relay to cut connection.
	// For MVP: We just remove from Core logic, so Next Join behaves as "Not Member".
	// Real-time kick requires Relay integration (Control Message).

	return &pbv1.KickResponse{Success: true, Message: "User kicked"}, nil
}

func (s *ChannelServerImpl) UpdateChannelAvatar(ctx context.Context, req *pbv1.UpdateChannelAvatarRequest) (*pbv1.UpdateChannelAvatarResponse, error) {
	// Verify Signature
	// Payload format: target|avatar|timestamp
	// Note: for avatar logic we use target = channelID
	payload := fmt.Sprintf("%s|%s|%d", req.ChannelId, req.Avatar, req.Timestamp)

	node, err := s.nodeService.GetNode(ctx, req.RequesterId)
	if err != nil {
		return &pbv1.UpdateChannelAvatarResponse{Success: false, Message: "Requester not found"}, nil
	}

	if !ed25519.Verify(ed25519.PublicKey(node.Identity.PublicKey), []byte(payload), req.Signature) {
		return &pbv1.UpdateChannelAvatarResponse{Success: false, Message: "Invalid signature"}, nil
	}

	val, ok := s.channels.Load(req.ChannelId)
	if !ok {
		return &pbv1.UpdateChannelAvatarResponse{Success: false, Message: "Channel not found"}, nil
	}
	ch := val.(*ChannelData)

	ch.mu.Lock()
	defer ch.mu.Unlock()

	// Permission Check
	role := ch.Roles[req.RequesterId]
	if role != pbv1.Role_ADMIN && ch.OwnerID != req.RequesterId {
		return &pbv1.UpdateChannelAvatarResponse{Success: false, Message: "Permission denied"}, nil
	}

	ch.Avatar = req.Avatar

	return &pbv1.UpdateChannelAvatarResponse{Success: true, Message: "Avatar updated"}, nil
}

func (s *ChannelServerImpl) verifyGovernanceSignature(ctx context.Context, requesterID string, sig []byte, targetID, role, scope, channelID string, ts int64) error {
	// 1. Get Node Public Key
	node, err := s.nodeService.GetNode(ctx, requesterID)
	if err != nil {
		return fmt.Errorf("requester not found: %v", err)
	}

	// 2. Reconstruct Payload
	// Format: target|role|scope|channel|timestamp
	payload := fmt.Sprintf("%s|%s|%s|%s|%d", targetID, role, scope, channelID, ts)

	// 3. Verify
	if !ed25519.Verify(ed25519.PublicKey(node.Identity.PublicKey), []byte(payload), sig) {
		return errors.New("invalid signature")
	}

	// 4. Verify Timestamp (Anti-Replay) - 60s window
	now := time.Now().UnixNano()
	diff := now - ts
	if diff < 0 {
		diff = -diff
	}
	if diff > (60 * time.Second).Nanoseconds() {
		return errors.New("signature expired")
	}

	return nil
}

// RegisterChannelService registra o serviço manualmente no gRPC Server
// NOME DO SERVIÇO: channel.v1.ChannelService
func RegisterChannelService(s *grpc.Server, srv interface{}) {
	desc := &grpc.ServiceDesc{
		ServiceName: "channel.v1.ChannelService",
		HandlerType: (*ChannelService)(nil),
		Methods: []grpc.MethodDesc{
			{
				MethodName: "CreateChannel",
				Handler:    _ChannelService_CreateChannel_Handler,
			},
			{
				MethodName: "GetChannelInfo", // Re-enabled
				Handler:    _ChannelService_GetChannelInfo_Handler,
			},
			{
				MethodName: "PromoteUser",
				Handler:    _ChannelService_PromoteUser_Handler,
			},
			{
				MethodName: "DemoteUser",
				Handler:    _ChannelService_DemoteUser_Handler,
			},
			{
				MethodName: "KickUser",
				Handler:    _ChannelService_KickUser_Handler,
			},
			{
				MethodName: "UpdateChannelAvatar",
				Handler:    _ChannelService_UpdateChannelAvatar_Handler,
			},
		},
		Streams:  []grpc.StreamDesc{},
		Metadata: "channel.proto",
	}
	s.RegisterService(desc, srv)
}

// Handlers Manuais (Boilerplate gRPC)

func _ChannelService_CreateChannel_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(pbv1.CreateChannelRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(ChannelService).CreateChannel(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/channel.v1.ChannelService/CreateChannel",
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(ChannelService).CreateChannel(ctx, req.(*pbv1.CreateChannelRequest))
	}
	return interceptor(ctx, in, info, handler)
}

// ... (Existing CreateChannel Handler)

func _ChannelService_GetChannelInfo_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(pbv1.GetChannelInfoRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(ChannelService).GetChannelInfo(ctx, in)
	}
	info := &grpc.UnaryServerInfo{Server: srv, FullMethod: "/channel.v1.ChannelService/GetChannelInfo"}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(ChannelService).GetChannelInfo(ctx, req.(*pbv1.GetChannelInfoRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _ChannelService_PromoteUser_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(pbv1.PromoteRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(ChannelService).PromoteUser(ctx, in)
	}
	info := &grpc.UnaryServerInfo{Server: srv, FullMethod: "/channel.v1.ChannelService/PromoteUser"}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(ChannelService).PromoteUser(ctx, req.(*pbv1.PromoteRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _ChannelService_DemoteUser_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(pbv1.DemoteRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(ChannelService).DemoteUser(ctx, in)
	}
	info := &grpc.UnaryServerInfo{Server: srv, FullMethod: "/channel.v1.ChannelService/DemoteUser"}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(ChannelService).DemoteUser(ctx, req.(*pbv1.DemoteRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _ChannelService_KickUser_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(pbv1.KickRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(ChannelService).KickUser(ctx, in)
	}
	info := &grpc.UnaryServerInfo{Server: srv, FullMethod: "/channel.v1.ChannelService/KickUser"}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(ChannelService).KickUser(ctx, req.(*pbv1.KickRequest))
	}
	return interceptor(ctx, in, info, handler)
}
func _ChannelService_UpdateChannelAvatar_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(pbv1.UpdateChannelAvatarRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(ChannelService).UpdateChannelAvatar(ctx, in)
	}
	info := &grpc.UnaryServerInfo{Server: srv, FullMethod: "/channel.v1.ChannelService/UpdateChannelAvatar"}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(ChannelService).UpdateChannelAvatar(ctx, req.(*pbv1.UpdateChannelAvatarRequest))
	}
	return interceptor(ctx, in, info, handler)
}
