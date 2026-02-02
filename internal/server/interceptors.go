package server

import (
	"context"
	"log"
	"runtime/debug"
	"sync"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// LoggingInterceptor registra todas as chamadas gRPC
func LoggingInterceptor(
	ctx context.Context,
	req interface{},
	info *grpc.UnaryServerInfo,
	handler grpc.UnaryHandler,
) (interface{}, error) {
	start := time.Now()

	resp, err := handler(ctx, req)

	duration := time.Since(start)

	if err != nil {
		log.Printf("[gRPC] %s | ERROR | %v | %s", info.FullMethod, duration, err.Error())
	} else {
		log.Printf("[gRPC] %s | OK | %v", info.FullMethod, duration)
	}

	return resp, err
}

// StreamLoggingInterceptor registra chamadas de streaming
func StreamLoggingInterceptor(
	srv interface{},
	ss grpc.ServerStream,
	info *grpc.StreamServerInfo,
	handler grpc.StreamHandler,
) error {
	start := time.Now()

	log.Printf("[gRPC Stream] %s | STARTED", info.FullMethod)

	err := handler(srv, ss)

	duration := time.Since(start)

	if err != nil {
		log.Printf("[gRPC Stream] %s | ENDED | %v | %s", info.FullMethod, duration, err.Error())
	} else {
		log.Printf("[gRPC Stream] %s | ENDED | %v", info.FullMethod, duration)
	}

	return err
}

// RecoveryInterceptor recupera de panics
func RecoveryInterceptor(
	ctx context.Context,
	req interface{},
	info *grpc.UnaryServerInfo,
	handler grpc.UnaryHandler,
) (resp interface{}, err error) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("[gRPC PANIC] %s: %v\n%s", info.FullMethod, r, debug.Stack())
			err = status.Errorf(codes.Internal, "internal server error")
		}
	}()

	return handler(ctx, req)
}

// StreamRecoveryInterceptor recupera de panics em streams
func StreamRecoveryInterceptor(
	srv interface{},
	ss grpc.ServerStream,
	info *grpc.StreamServerInfo,
	handler grpc.StreamHandler,
) (err error) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("[gRPC Stream PANIC] %s: %v\n%s", info.FullMethod, r, debug.Stack())
			err = status.Errorf(codes.Internal, "internal server error")
		}
	}()

	return handler(srv, ss)
}

// RateLimiter implementa rate limiting thread-safe por nó
type RateLimiter struct {
	mu       sync.RWMutex
	requests map[string][]time.Time
	limit    int
	window   time.Duration
}

// NewRateLimiter cria um novo rate limiter
func NewRateLimiter(limit int, window time.Duration) *RateLimiter {
	rl := &RateLimiter{
		requests: make(map[string][]time.Time),
		limit:    limit,
		window:   window,
	}

	// Goroutine para limpeza periódica de entradas antigas
	go rl.cleanup()

	return rl
}

// Allow verifica se uma request é permitida (thread-safe)
func (rl *RateLimiter) Allow(key string) bool {
	now := time.Now()
	windowStart := now.Add(-rl.window)

	rl.mu.Lock()
	defer rl.mu.Unlock()

	// Filtra requests dentro da janela
	var recent []time.Time
	for _, t := range rl.requests[key] {
		if t.After(windowStart) {
			recent = append(recent, t)
		}
	}

	if len(recent) >= rl.limit {
		return false
	}

	rl.requests[key] = append(recent, now)
	return true
}

// cleanup remove entradas antigas periodicamente
func (rl *RateLimiter) cleanup() {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		rl.mu.Lock()
		now := time.Now()
		windowStart := now.Add(-rl.window)

		for key, times := range rl.requests {
			var recent []time.Time
			for _, t := range times {
				if t.After(windowStart) {
					recent = append(recent, t)
				}
			}
			if len(recent) == 0 {
				delete(rl.requests, key)
			} else {
				rl.requests[key] = recent
			}
		}
		rl.mu.Unlock()
	}
}

// ChainUnaryInterceptors combina múltiplos interceptors
func ChainUnaryInterceptors(interceptors ...grpc.UnaryServerInterceptor) grpc.UnaryServerInterceptor {
	return func(
		ctx context.Context,
		req interface{},
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (interface{}, error) {
		// Constrói a cadeia de interceptors
		chain := handler
		for i := len(interceptors) - 1; i >= 0; i-- {
			interceptor := interceptors[i]
			next := chain
			chain = func(ctx context.Context, req interface{}) (interface{}, error) {
				return interceptor(ctx, req, info, next)
			}
		}
		return chain(ctx, req)
	}
}

// ChainStreamInterceptors combina múltiplos interceptors de stream
func ChainStreamInterceptors(interceptors ...grpc.StreamServerInterceptor) grpc.StreamServerInterceptor {
	return func(
		srv interface{},
		ss grpc.ServerStream,
		info *grpc.StreamServerInfo,
		handler grpc.StreamHandler,
	) error {
		chain := handler
		for i := len(interceptors) - 1; i >= 0; i-- {
			interceptor := interceptors[i]
			next := chain
			chain = func(srv interface{}, ss grpc.ServerStream) error {
				return interceptor(srv, ss, info, next)
			}
		}
		return chain(srv, ss)
	}
}
