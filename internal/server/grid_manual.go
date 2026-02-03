package server

import (
	"context"
	"fmt"
	"log"
	"time"

	"google.golang.org/grpc"

	pbv1 "github.com/goautomatik/core-server/pkg/pb/v1"
)

// -- Implementação do Servidor Grid (MVP) --

type GridServerImpl struct {
	// Poderia ter QueueService, etc.
}

func NewGridServer() *GridServerImpl {
	return &GridServerImpl{}
}

func (s *GridServerImpl) SubscribeToJobs(in *pbv1.NodeIdentity, stream pbv1.GridService_SubscribeToJobsServer) error {
	log.Printf("[Grid] Node %s subscribed for jobs", in.NodeId)

	// MVP: Send 1 simulation job after 5 seconds
	go func() {
		time.Sleep(5 * time.Second)
		job := &pbv1.GridJob{
			JobId:     fmt.Sprintf("job-%d", time.Now().Unix()),
			Payload:   "SIMULATE_COMPUTATION_X",
			Requester: "CORE_SYSTEM",
			Timestamp: time.Now().Unix(),
		}
		if err := stream.Send(job); err != nil {
			log.Printf("Failed to send job to %s: %v", in.NodeId, err)
		} else {
			log.Printf("Sent job %s to %s", job.JobId, in.NodeId)
		}
	}()

	// Keep stream open
	<-stream.Context().Done()
	return nil
}

func (s *GridServerImpl) SubmitResult(ctx context.Context, in *pbv1.SubmitJobResultRequest) (*pbv1.SubmitJobResultResponse, error) {
	log.Printf("[Grid] Received Result for %s from %s: Success=%v, Output=%s", in.JobId, in.WorkerId, in.Success, in.Result)
	return &pbv1.SubmitJobResultResponse{Success: true, Message: "Result Accepted"}, nil
}

// RegisterGridService registra o serviço manualmente
func RegisterGridService(s *grpc.Server, srv interface{}) {
	// Boilerplate manual registration
	desc := &grpc.ServiceDesc{
		ServiceName: "channel.v1.GridService",
		HandlerType: (*pbv1.GridServiceServer)(nil),
		Methods: []grpc.MethodDesc{
			{
				MethodName: "SubmitResult",
				Handler:    _GridService_SubmitResult_Handler(srv),
			},
		},
		Streams: []grpc.StreamDesc{
			{
				StreamName:    "SubscribeToJobs",
				Handler:       _GridService_SubscribeToJobs_Handler(srv),
				ServerStreams: true,
			},
		},
		Metadata: "manual_extras.go",
	}
	s.RegisterService(desc, srv)
}

func _GridService_SubmitResult_Handler(srv interface{}) func(interface{}, context.Context, func(interface{}) error, grpc.UnaryServerInterceptor) (interface{}, error) {
	return func(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
		in := new(pbv1.SubmitJobResultRequest)
		if err := dec(in); err != nil {
			return nil, err
		}
		if interceptor == nil {
			return srv.(pbv1.GridServiceServer).SubmitResult(ctx, in)
		}
		info := &grpc.UnaryServerInfo{
			Server:     srv,
			FullMethod: "/channel.v1.GridService/SubmitResult",
		}
		handler := func(ctx context.Context, req interface{}) (interface{}, error) {
			return srv.(pbv1.GridServiceServer).SubmitResult(ctx, req.(*pbv1.SubmitJobResultRequest))
		}
		return interceptor(ctx, in, info, handler)
	}
}

func _GridService_SubscribeToJobs_Handler(srv interface{}) func(interface{}, grpc.ServerStream) error {
	return func(srv interface{}, stream grpc.ServerStream) error {
		m := new(pbv1.NodeIdentity)
		if err := stream.RecvMsg(m); err != nil {
			return err
		}
		return srv.(pbv1.GridServiceServer).SubscribeToJobs(m, &gridServiceSubscribeToJobsServer{stream})
	}
}

type gridServiceSubscribeToJobsServer struct {
	grpc.ServerStream
}

func (x *gridServiceSubscribeToJobsServer) Send(m *pbv1.GridJob) error {
	return x.ServerStream.SendMsg(m)
}
