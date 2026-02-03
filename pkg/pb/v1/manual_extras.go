package pbv1

import (
	"context"

	"google.golang.org/grpc"
)

// Estruturas Manuais para ChannelService (já que não podemos rodar protoc)

type CreateChannelRequest struct {
	Name        string `protobuf:"bytes,1,opt,name=name,proto3" json:"name,omitempty"`
	OwnerNodeId string `protobuf:"bytes,2,opt,name=owner_node_id,json=ownerNodeId,proto3" json:"owner_node_id,omitempty"`
	Avatar      string `protobuf:"bytes,3,opt,name=avatar,proto3" json:"avatar,omitempty"`
}

type CreateChannelResponse struct {
	Success   bool   `protobuf:"varint,1,opt,name=success,proto3" json:"success,omitempty"`
	Message   string `protobuf:"bytes,2,opt,name=message,proto3" json:"message,omitempty"`
	ChannelId string `protobuf:"bytes,3,opt,name=channel_id,json=channelId,proto3" json:"channel_id,omitempty"`
}

type GetChannelInfoRequest struct {
	ChannelId string `protobuf:"bytes,1,opt,name=channel_id,proto3" json:"channel_id,omitempty"`
}

type GetChannelInfoResponse struct {
	ChannelId string `protobuf:"bytes,1,opt,name=channel_id,proto3" json:"channel_id,omitempty"`
	Name      string `protobuf:"bytes,2,opt,name=name,proto3" json:"name,omitempty"`
	OwnerId   string `protobuf:"bytes,3,opt,name=owner_id,json=ownerId,proto3" json:"owner_id,omitempty"`
	Avatar    string `protobuf:"bytes,4,opt,name=avatar,proto3" json:"avatar,omitempty"`
}

// Governance Types
const (
	Role_MEMBER    = "MEMBER"
	Role_MODERATOR = "MODERATOR"
	Role_ADMIN     = "ADMIN"

	Scope_CHANNEL = "CHANNEL"
	Scope_GLOBAL  = "GLOBAL"
)

type PromoteRequest struct {
	TargetNodeId string `json:"target_node_id"`
	Role         string `json:"role"`
	Scope        string `json:"scope"`
	ChannelId    string `json:"channel_id,omitempty"` // Required if Scope=CHANNEL
	RequesterId  string `json:"requester_id"`
	Timestamp    int64  `json:"timestamp"` // Unix Nano
	Signature    []byte `json:"signature"` // Signed params by Requester private key
}

type PromoteResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
}

type DemoteRequest struct {
	TargetNodeId string `json:"target_node_id"`
	Scope        string `json:"scope"`
	ChannelId    string `json:"channel_id,omitempty"`
	RequesterId  string `json:"requester_id"`
	Timestamp    int64  `json:"timestamp"`
	Signature    []byte `json:"signature"`
}

type DemoteResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
}

type KickRequest struct {
	TargetNodeId string `json:"target_node_id"`
	Scope        string `json:"scope"`
	ChannelId    string `json:"channel_id,omitempty"`
	RequesterId  string `json:"requester_id"`
	Timestamp    int64  `json:"timestamp"`
	Signature    []byte `json:"signature"`
}

type KickResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
}

type UpdateChannelAvatarRequest struct {
	ChannelId   string `json:"channel_id"`
	Avatar      string `json:"avatar"`
	RequesterId string `json:"requester_id"`
	Timestamp   int64  `json:"timestamp"`
	Signature   []byte `json:"signature"`
}

type UpdateChannelAvatarResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
}

// ChannelServiceClient Interface
type ChannelServiceClient interface {
	CreateChannel(ctx context.Context, in *CreateChannelRequest, opts ...grpc.CallOption) (*CreateChannelResponse, error)
	GetChannelInfo(ctx context.Context, in *GetChannelInfoRequest, opts ...grpc.CallOption) (*GetChannelInfoResponse, error)
	PromoteUser(ctx context.Context, in *PromoteRequest, opts ...grpc.CallOption) (*PromoteResponse, error)
	DemoteUser(ctx context.Context, in *DemoteRequest, opts ...grpc.CallOption) (*DemoteResponse, error)
	KickUser(ctx context.Context, in *KickRequest, opts ...grpc.CallOption) (*KickResponse, error)
	UpdateChannelAvatar(ctx context.Context, in *UpdateChannelAvatarRequest, opts ...grpc.CallOption) (*UpdateChannelAvatarResponse, error)
}

type channelServiceClient struct {
	cc grpc.ClientConnInterface
}

func NewChannelServiceClient(cc grpc.ClientConnInterface) ChannelServiceClient {
	return &channelServiceClient{cc}
}

func (c *channelServiceClient) CreateChannel(ctx context.Context, in *CreateChannelRequest, opts ...grpc.CallOption) (*CreateChannelResponse, error) {
	out := new(CreateChannelResponse)
	err := c.cc.Invoke(ctx, "/channel.v1.ChannelService/CreateChannel", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *channelServiceClient) GetChannelInfo(ctx context.Context, in *GetChannelInfoRequest, opts ...grpc.CallOption) (*GetChannelInfoResponse, error) {
	out := new(GetChannelInfoResponse)
	err := c.cc.Invoke(ctx, "/channel.v1.ChannelService/GetChannelInfo", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *channelServiceClient) PromoteUser(ctx context.Context, in *PromoteRequest, opts ...grpc.CallOption) (*PromoteResponse, error) {
	out := new(PromoteResponse)
	err := c.cc.Invoke(ctx, "/channel.v1.ChannelService/PromoteUser", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *channelServiceClient) DemoteUser(ctx context.Context, in *DemoteRequest, opts ...grpc.CallOption) (*DemoteResponse, error) {
	out := new(DemoteResponse)
	err := c.cc.Invoke(ctx, "/channel.v1.ChannelService/DemoteUser", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *channelServiceClient) KickUser(ctx context.Context, in *KickRequest, opts ...grpc.CallOption) (*KickResponse, error) {
	out := new(KickResponse)
	err := c.cc.Invoke(ctx, "/channel.v1.ChannelService/KickUser", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *channelServiceClient) UpdateChannelAvatar(ctx context.Context, in *UpdateChannelAvatarRequest, opts ...grpc.CallOption) (*UpdateChannelAvatarResponse, error) {
	out := new(UpdateChannelAvatarResponse)
	err := c.cc.Invoke(ctx, "/channel.v1.ChannelService/UpdateChannelAvatar", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

// -- GRID SERVICE (MVP) --

type GridJob struct {
	JobId     string `json:"job_id"`
	Payload   string `json:"payload"`   // Command or Data
	Requester string `json:"requester"` // NodeID
	Timestamp int64  `json:"timestamp"`
}

type SubmitJobResultRequest struct {
	JobId    string `json:"job_id"`
	WorkerId string `json:"worker_id"`
	Result   string `json:"result"` // Output
	Success  bool   `json:"success"`
}

type SubmitJobResultResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
}

type GridServiceClient interface {
	SubscribeToJobs(ctx context.Context, in *NodeIdentity, opts ...grpc.CallOption) (GridService_SubscribeToJobsClient, error)
	SubmitResult(ctx context.Context, in *SubmitJobResultRequest, opts ...grpc.CallOption) (*SubmitJobResultResponse, error)
}

type GridServiceServer interface {
	SubscribeToJobs(*NodeIdentity, GridService_SubscribeToJobsServer) error
	SubmitResult(context.Context, *SubmitJobResultRequest) (*SubmitJobResultResponse, error)
}

// Stream Interfaces
type GridService_SubscribeToJobsClient interface {
	Recv() (*GridJob, error)
	grpc.ClientStream
}

type GridService_SubscribeToJobsServer interface {
	Send(*GridJob) error
	grpc.ServerStream
}

type gridServiceSubscribeToJobsClient struct {
	grpc.ClientStream
}

func (x *gridServiceSubscribeToJobsClient) Recv() (*GridJob, error) {
	m := new(GridJob)
	if err := x.ClientStream.RecvMsg(m); err != nil {
		return nil, err
	}
	return m, nil
}

// Client Implementation
type gridServiceClient struct {
	cc grpc.ClientConnInterface
}

func NewGridServiceClient(cc grpc.ClientConnInterface) GridServiceClient {
	return &gridServiceClient{cc}
}

func (c *gridServiceClient) SubscribeToJobs(ctx context.Context, in *NodeIdentity, opts ...grpc.CallOption) (GridService_SubscribeToJobsClient, error) {
	stream, err := c.cc.NewStream(ctx, &_GridService_serviceDesc.Streams[0], "/channel.v1.GridService/SubscribeToJobs", opts...)
	if err != nil {
		return nil, err
	}
	x := &gridServiceSubscribeToJobsClient{stream}
	if err := x.ClientStream.SendMsg(in); err != nil {
		return nil, err
	}
	if err := x.ClientStream.CloseSend(); err != nil {
		return nil, err
	}
	return x, nil
}

func (c *gridServiceClient) SubmitResult(ctx context.Context, in *SubmitJobResultRequest, opts ...grpc.CallOption) (*SubmitJobResultResponse, error) {
	out := new(SubmitJobResultResponse)
	err := c.cc.Invoke(ctx, "/channel.v1.GridService/SubmitResult", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

// Server Boilerplate (Metadata)
var _GridService_serviceDesc = grpc.ServiceDesc{
	ServiceName: "channel.v1.GridService",
	HandlerType: (*GridServiceServer)(nil),
	Methods: []grpc.MethodDesc{
		{
			MethodName: "SubmitResult",
			Handler:    nil, // Filled manually in implementation
		},
	},
	Streams: []grpc.StreamDesc{
		{
			StreamName:    "SubscribeToJobs",
			Handler:       nil, // Filled manually in implementation
			ServerStreams: true,
			ClientStreams: false,
		},
	},
	Metadata: "manual_extras.go",
}

// -- CLUSTER SERVICE (High Availability) --

type PeerInfo struct {
	PeerID    string `json:"peer_id"`
	Address   string `json:"address"`    // IP:Port
	PublicKey []byte `json:"public_key"` // Ed25519 public key
}

type RequestJoinRequest struct {
	Peer *PeerInfo `json:"peer"`
}

type RequestJoinResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
}

type ApprovePeerRequest struct {
	PeerID      string `json:"peer_id"`
	RequesterId string `json:"requester_id"` // Admin who approves
	Timestamp   int64  `json:"timestamp"`
	Signature   []byte `json:"signature"`
}

type ApprovePeerResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
}

type GetClusterNodesRequest struct{}

type GetClusterNodesResponse struct {
	Nodes []*PeerInfo `json:"nodes"`
}

// SyncNodesRequest is sent from primary to new peer to dump existing users
type SyncNodesRequest struct {
	Nodes []*NodeIdentity `json:"nodes"`
}

type SyncNodesResponse struct {
	Success bool  `json:"success"`
	Count   int32 `json:"count"`
}

// ClusterServiceClient Interface
type ClusterServiceClient interface {
	RequestJoin(ctx context.Context, in *RequestJoinRequest, opts ...grpc.CallOption) (*RequestJoinResponse, error)
	ApprovePeer(ctx context.Context, in *ApprovePeerRequest, opts ...grpc.CallOption) (*ApprovePeerResponse, error)
	GetClusterNodes(ctx context.Context, in *GetClusterNodesRequest, opts ...grpc.CallOption) (*GetClusterNodesResponse, error)
	SyncNodes(ctx context.Context, in *SyncNodesRequest, opts ...grpc.CallOption) (*SyncNodesResponse, error)
}

type clusterServiceClient struct {
	cc grpc.ClientConnInterface
}

func NewClusterServiceClient(cc grpc.ClientConnInterface) ClusterServiceClient {
	return &clusterServiceClient{cc}
}

func (c *clusterServiceClient) RequestJoin(ctx context.Context, in *RequestJoinRequest, opts ...grpc.CallOption) (*RequestJoinResponse, error) {
	out := new(RequestJoinResponse)
	err := c.cc.Invoke(ctx, "/cluster.v1.ClusterService/RequestJoin", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *clusterServiceClient) ApprovePeer(ctx context.Context, in *ApprovePeerRequest, opts ...grpc.CallOption) (*ApprovePeerResponse, error) {
	out := new(ApprovePeerResponse)
	err := c.cc.Invoke(ctx, "/cluster.v1.ClusterService/ApprovePeer", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *clusterServiceClient) GetClusterNodes(ctx context.Context, in *GetClusterNodesRequest, opts ...grpc.CallOption) (*GetClusterNodesResponse, error) {
	out := new(GetClusterNodesResponse)
	err := c.cc.Invoke(ctx, "/cluster.v1.ClusterService/GetClusterNodes", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *clusterServiceClient) SyncNodes(ctx context.Context, in *SyncNodesRequest, opts ...grpc.CallOption) (*SyncNodesResponse, error) {
	out := new(SyncNodesResponse)
	err := c.cc.Invoke(ctx, "/cluster.v1.ClusterService/SyncNodes", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}
