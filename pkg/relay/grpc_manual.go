package relay

import (
	"context"

	pbv1 "github.com/goautomatik/core-server/pkg/pb/v1"
	"google.golang.org/grpc"
)

// RelayServiceServer é a interface que o servidor deve implementar
// Manualmente definida pois não temos o código gerado pelo protoc
type RelayServiceServer interface {
	StreamMessages(RelayService_StreamMessagesServer) error
	ConnectToRelay(context.Context, *pbv1.ConnectRequest) (*pbv1.ConnectResponse, error)
	IsNodeConnected(context.Context, *pbv1.NodeQueryRequest) (*pbv1.NodeQueryResponse, error)
}

// RelayService_StreamMessagesServer interface para o stream
type RelayService_StreamMessagesServer interface {
	Send(*pbv1.NodeCommand) error
	Recv() (*pbv1.NodeCommand, error)
	grpc.ServerStream
}

// Descrição do serviço para registro manual
var RelayService_ServiceDesc = grpc.ServiceDesc{
	ServiceName: "relay.v1.RelayService",
	HandlerType: (*RelayServiceServer)(nil),
	Methods: []grpc.MethodDesc{
		{
			MethodName: "ConnectToRelay",
			Handler:    _RelayService_ConnectToRelay_Handler,
		},
		{
			MethodName: "IsNodeConnected",
			Handler:    _RelayService_IsNodeConnected_Handler,
		},
	},
	Streams: []grpc.StreamDesc{
		{
			StreamName:    "StreamMessages",
			Handler:       _RelayService_StreamMessages_Handler,
			ServerStreams: true,
			ClientStreams: true,
		},
	},
	Metadata: "relay.proto",
}

// Handlers manuais
func _RelayService_ConnectToRelay_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(pbv1.ConnectRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(RelayServiceServer).ConnectToRelay(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/relay.v1.RelayService/ConnectToRelay",
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(RelayServiceServer).ConnectToRelay(ctx, req.(*pbv1.ConnectRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _RelayService_IsNodeConnected_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(pbv1.NodeQueryRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(RelayServiceServer).IsNodeConnected(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/relay.v1.RelayService/IsNodeConnected",
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(RelayServiceServer).IsNodeConnected(ctx, req.(*pbv1.NodeQueryRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _RelayService_StreamMessages_Handler(srv interface{}, stream grpc.ServerStream) error {
	return srv.(RelayServiceServer).StreamMessages(&relayServiceStreamMessagesServer{stream})
}

type relayServiceStreamMessagesServer struct {
	grpc.ServerStream
}

func (x *relayServiceStreamMessagesServer) Send(m *pbv1.NodeCommand) error {
	return x.ServerStream.SendMsg(m)
}

func (x *relayServiceStreamMessagesServer) Recv() (*pbv1.NodeCommand, error) {
	m := new(pbv1.NodeCommand)
	if err := x.ServerStream.RecvMsg(m); err != nil {
		return nil, err
	}
	return m, nil
}

// RegisterRelayServiceServer registra o serviço no servidor gRPC
func RegisterRelayServiceServer(s grpc.ServiceRegistrar, srv RelayServiceServer) {
	s.RegisterService(&RelayService_ServiceDesc, srv)
}
