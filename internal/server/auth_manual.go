package server

import (
	"context"
	"log"

	"github.com/goautomatik/core-server/internal/crypto"
	"github.com/goautomatik/core-server/internal/repository/postgres"
	pbv1 "github.com/goautomatik/core-server/pkg/pb/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type AuthServer struct {
	repo *postgres.UserRepository
}

func NewAuthServer(repo *postgres.UserRepository) *AuthServer {
	return &AuthServer{repo: repo}
}

func (s *AuthServer) Register(ctx context.Context, req *pbv1.RegisterRequest) (*pbv1.RegisterResponse, error) {
	if req.Email == "" || req.Password == "" || req.Nickname == "" {
		return nil, status.Error(codes.InvalidArgument, "email, password and nickname are required")
	}

	hash, err := crypto.HashPassword(req.Password)
	if err != nil {
		return nil, status.Error(codes.Internal, "failed to hash password")
	}

	user := &postgres.User{
		Email:        req.Email,
		PasswordHash: hash,
		FullName:     req.FullName,
		Phone:        req.Phone,
		Nickname:     req.Nickname,
	}

	created, err := s.repo.CreateUser(ctx, user)
	if err != nil {
		log.Printf("[Auth] Failed to register user %s: %v", req.Email, err)
		return nil, status.Errorf(codes.Internal, "failed to create user: %v", err)
	}

	token, err := crypto.GenerateJWT(created.ID, created.Role)
	if err != nil {
		return nil, status.Error(codes.Internal, "failed to generate token")
	}

	log.Printf("[Auth] User registered: %s (Role: %s)", created.Email, created.Role)

	return &pbv1.RegisterResponse{
		Success: true,
		Message: "Registered successfully",
		Token:   token,
		User: &pbv1.User{
			Id:       created.ID,
			Email:    created.Email,
			FullName: created.FullName,
			Nickname: created.Nickname,
			Role:     created.Role,
		},
	}, nil
}

func (s *AuthServer) Login(ctx context.Context, req *pbv1.LoginRequest) (*pbv1.LoginResponse, error) {
	user, err := s.repo.GetUserByEmail(ctx, req.Email)
	if err != nil {
		return nil, status.Error(codes.Internal, "database error")
	}
	if user == nil {
		return nil, status.Error(codes.Unauthenticated, "invalid credentials")
	}

	if !crypto.CheckPasswordHash(req.Password, user.PasswordHash) {
		return nil, status.Error(codes.Unauthenticated, "invalid credentials")
	}

	token, err := crypto.GenerateJWT(user.ID, user.Role)
	if err != nil {
		return nil, status.Error(codes.Internal, "failed to generate token")
	}

	log.Printf("[Auth] User logged in: %s", user.Email)

	return &pbv1.LoginResponse{
		Success: true,
		Token:   token,
		User: &pbv1.User{
			Id:       user.ID,
			Email:    user.Email,
			FullName: user.FullName,
			Nickname: user.Nickname,
			Role:     user.Role,
		},
	}, nil
}

func (s *AuthServer) GetMe(ctx context.Context, req *pbv1.GetMeRequest) (*pbv1.GetMeResponse, error) {
	// O interceptor de Auth deve injetar o UserID no contexto
	userID, ok := ctx.Value("user_id").(string)
	if !ok {
		return nil, status.Error(codes.Unauthenticated, "not authenticated")
	}

	user, err := s.repo.GetUserByID(ctx, userID)
	if err != nil {
		return nil, status.Error(codes.Internal, "database error")
	}
	if user == nil {
		return nil, status.Error(codes.NotFound, "user not found")
	}

	return &pbv1.GetMeResponse{
		Success: true,
		User: &pbv1.User{
			Id:       user.ID,
			Email:    user.Email,
			FullName: user.FullName,
			Nickname: user.Nickname,
			Role:     user.Role,
		},
	}, nil
}

// Register as an extra service
func (s *AuthServer) RegisterGRPC(grpcServer *grpc.Server) {
	// Registro manual do AuthService
	grpcServer.RegisterService(&_AuthService_serviceDesc, s)
}

// -- gRPC Service Descriptor (Manual) --

var _AuthService_serviceDesc = grpc.ServiceDesc{
	ServiceName: "auth.v1.AuthService",
	HandlerType: (*pbv1.AuthServiceServer)(nil),
	Methods: []grpc.MethodDesc{
		{MethodName: "Login", Handler: _AuthService_Login_Handler},
		{MethodName: "Register", Handler: _AuthService_Register_Handler},
		{MethodName: "GetMe", Handler: _AuthService_GetMe_Handler},
	},
	Streams:  []grpc.StreamDesc{},
	Metadata: "auth_manual.go",
}

func _AuthService_Login_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(pbv1.LoginRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(*AuthServer).Login(ctx, in)
	}
	info := &grpc.UnaryServerInfo{Server: srv, FullMethod: "/auth.v1.AuthService/Login"}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(*AuthServer).Login(ctx, req.(*pbv1.LoginRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _AuthService_Register_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(pbv1.RegisterRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(*AuthServer).Register(ctx, in)
	}
	info := &grpc.UnaryServerInfo{Server: srv, FullMethod: "/auth.v1.AuthService/Register"}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(*AuthServer).Register(ctx, req.(*pbv1.RegisterRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _AuthService_GetMe_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(pbv1.GetMeRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(*AuthServer).GetMe(ctx, in)
	}
	info := &grpc.UnaryServerInfo{Server: srv, FullMethod: "/auth.v1.AuthService/GetMe"}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(*AuthServer).GetMe(ctx, req.(*pbv1.GetMeRequest))
	}
	return interceptor(ctx, in, info, handler)
}
