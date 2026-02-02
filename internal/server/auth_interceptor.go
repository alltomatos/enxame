package server

import (
	"context"
	"log"
	"strings"

	"github.com/goautomatik/core-server/internal/crypto"
	pbv1 "github.com/goautomatik/core-server/pkg/pb/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// AuthInterceptor valida assinaturas Ed25519 em chamadas que requerem autenticação
type AuthInterceptor struct {
	verifier *crypto.Verifier
}

// NewAuthInterceptor cria um novo interceptor de autenticação
func NewAuthInterceptor(verifier *crypto.Verifier) *AuthInterceptor {
	return &AuthInterceptor{
		verifier: verifier,
	}
}

// Unary retorna o interceptor unário para autenticação
func (a *AuthInterceptor) Unary() grpc.UnaryServerInterceptor {
	return func(
		ctx context.Context,
		req interface{},
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (interface{}, error) {
		// Apenas RegisterNode requer autenticação de identidade
		if strings.HasSuffix(info.FullMethod, "/RegisterNode") {
			if err := a.validateRegisterNode(req); err != nil {
				log.Printf("[Auth] RegisterNode failed: %v", err)
				return nil, err
			}
		}

		return handler(ctx, req)
	}
}

// Stream retorna o interceptor de stream para autenticação
func (a *AuthInterceptor) Stream() grpc.StreamServerInterceptor {
	return func(
		srv interface{},
		ss grpc.ServerStream,
		info *grpc.StreamServerInfo,
		handler grpc.StreamHandler,
	) error {
		// SubscribeToGlobalEvents - valida nodeID no request inicial
		// A validação é feita no handler pois precisamos do request
		return handler(srv, ss)
	}
}

// validateRegisterNode valida a identidade do nó em RegisterNode
func (a *AuthInterceptor) validateRegisterNode(req interface{}) error {
	registerReq, ok := req.(*pbv1.RegisterNodeRequest)
	if !ok {
		return status.Error(codes.Internal, "invalid request type")
	}

	identity := registerReq.GetIdentity()
	if identity == nil {
		return status.Error(codes.InvalidArgument, "identity is required")
	}

	nodeID := identity.GetNodeId()
	publicKey := identity.GetPublicKey()
	signature := identity.GetSignature()

	// Validação 1: Campos obrigatórios
	if nodeID == "" {
		return status.Error(codes.InvalidArgument, "node_id is required")
	}
	if len(publicKey) == 0 {
		return status.Error(codes.InvalidArgument, "public_key is required")
	}
	if len(signature) == 0 {
		return status.Error(codes.InvalidArgument, "signature is required")
	}

	// Validação 2: NodeID deve ser derivado da chave pública
	expectedNodeID := crypto.DeriveNodeID(publicKey)
	if nodeID != expectedNodeID {
		return status.Errorf(codes.InvalidArgument,
			"node_id does not match public key: expected %s, got %s",
			expectedNodeID, nodeID)
	}

	// Validação 3: Assinatura do nodeID deve ser válida
	if err := crypto.VerifySignature(publicKey, []byte(nodeID), signature); err != nil {
		return status.Errorf(codes.Unauthenticated,
			"signature verification failed: %v", err)
	}

	log.Printf("[Auth] Node %s authenticated successfully", nodeID)
	return nil
}
