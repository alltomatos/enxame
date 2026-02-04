package server

import (
	"context"
	"log"
	"strings"

	"github.com/goautomatik/core-server/internal/crypto"
	pbv1 "github.com/goautomatik/core-server/pkg/pb/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
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
		// 1. Identidade do Nó (Ed25519) - Apenas para RegisterNode (Segurança da Rede)
		if strings.HasSuffix(info.FullMethod, "/RegisterNode") {
			if err := a.validateRegisterNode(req); err != nil {
				return nil, err
			}
		}

		// 2. Identidade do Usuário (JWT) - (Segurança da Governança)
		newCtx := a.authenticateUser(ctx)

		// 3. Regras de RBAC
		if err := a.authorize(newCtx, info.FullMethod, req); err != nil {
			return nil, err
		}

		return handler(newCtx, req)
	}
}

func (a *AuthInterceptor) authenticateUser(ctx context.Context) context.Context {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return ctx
	}

	authHeader := md.Get("authorization")
	if len(authHeader) == 0 {
		return context.WithValue(ctx, "role", "guest")
	}

	tokenStr := strings.TrimPrefix(authHeader[0], "Bearer ")
	claims, err := crypto.ValidateJWT(tokenStr)
	if err != nil {
		log.Printf("[Auth] JWT validation failed: %v", err)
		return context.WithValue(ctx, "role", "guest")
	}

	ctx = context.WithValue(ctx, "user_id", claims.UserID)
	ctx = context.WithValue(ctx, "role", claims.Role)

	// Verificação do Vault Token (Opcional, apenas em certos métodos)
	vaultHeader := md.Get("x-vault-token")
	if len(vaultHeader) > 0 {
		vTokenStr := vaultHeader[0]
		vClaims, err := crypto.ValidateJWT(vTokenStr)
		if err == nil && vClaims.Scope == "super_admin" {
			ctx = context.WithValue(ctx, "vault_scope", "super_admin")
		}
	}

	return ctx
}

func (a *AuthInterceptor) authorize(ctx context.Context, method string, req interface{}) error {
	role, _ := ctx.Value("role").(string)
	if role == "" {
		role = "guest"
	}

	// Regra 1: Guests só podem ler mensagens no canal #Inicio
	if role == "guest" {
		if strings.HasSuffix(method, "/SendMessage") {
			return status.Error(codes.PermissionDenied, "guests cannot send messages")
		}
		if strings.HasSuffix(method, "/CreateChannel") {
			return status.Error(codes.PermissionDenied, "guests cannot create channels")
		}
		if strings.HasSuffix(method, "/GetMessages") {
			// Validar se o canal é #Inicio no request
			// Como o GetMessagesRequest é genérico aqui, precisaríamos de type assertion
			// mas por simplicidade desta fase, vamos focar no SendMessage e CreateChannel
		}
	}

	// Regra 2: Apenas Admin/Owner criam canais ou acessam AdminService
	if strings.Contains(method, "/AdminService/") || strings.HasSuffix(method, "/CreateChannel") {
		if role != "admin" && role != "owner" {
			return status.Error(codes.PermissionDenied, "unauthorized: only admins or owners can access this resource")
		}

		// Regra 3: Ações Críticas exigem Vault Scope (Modo Sudo)
		criticalMethods := []string{
			"BanNode",
			"UnbanNode",
			"SendGlobalAlert",
			"UpdateUserRole",
		}
		for _, m := range criticalMethods {
			if strings.HasSuffix(method, "/"+m) {
				vaultScope, _ := ctx.Value("vault_scope").(string)
				if vaultScope != "super_admin" {
					return status.Error(codes.Unauthenticated, "vault_access_required: this action requires master key unlock (vault mode)")
				}
			}
		}
	}

	return nil
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
