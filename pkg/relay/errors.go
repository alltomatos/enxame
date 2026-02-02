package relay

import "errors"

var (
	// ErrNodeBlacklisted indica que o nó está na blacklist global
	ErrNodeBlacklisted = errors.New("node is blacklisted")

	// ErrTargetNotConnected indica que o destinatário não está conectado a este Relay
	ErrTargetNotConnected = errors.New("target node not connected to this relay")

	// ErrBufferFull indica que o buffer de mensagens do destinatário está cheio
	ErrBufferFull = errors.New("message buffer full")

	// ErrInvalidSignature indica assinatura inválida
	ErrInvalidSignature = errors.New("invalid signature")

	// ErrSessionNotFound indica sessão não encontrada
	ErrSessionNotFound = errors.New("session not found")
)
