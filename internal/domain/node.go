package domain

import (
	"time"
)

// NodeType representa o tipo de nó na rede
type NodeType int

const (
	NodeTypeUnspecified NodeType = iota
	NodeTypeDesktop              // Cliente desktop P2P
	NodeTypeRelay                // Servidor relay da comunidade
	NodeTypeMobile               // Cliente mobile
	NodeTypeWeb                  // Cliente web
)

func (t NodeType) String() string {
	switch t {
	case NodeTypeDesktop:
		return "desktop"
	case NodeTypeRelay:
		return "relay"
	case NodeTypeMobile:
		return "mobile"
	case NodeTypeWeb:
		return "web"
	default:
		return "unspecified"
	}
}

// NodeStatus representa o status atual do nó
type NodeStatus int

const (
	NodeStatusUnspecified NodeStatus = iota
	NodeStatusOnline
	NodeStatusAway
	NodeStatusBusy
	NodeStatusOffline
)

func (s NodeStatus) String() string {
	switch s {
	case NodeStatusOnline:
		return "online"
	case NodeStatusAway:
		return "away"
	case NodeStatusBusy:
		return "busy"
	case NodeStatusOffline:
		return "offline"
	default:
		return "unspecified"
	}
}

// NodeIdentity representa a identidade criptográfica do nó
type NodeIdentity struct {
	// ID único derivado da chave pública (hex SHA256 primeiros 16 bytes)
	NodeID string

	// Chave pública Ed25519 (32 bytes)
	PublicKey []byte

	// Assinatura do NodeID pela chave privada
	Signature []byte
}

// Node representa um nó na rede P2P
type Node struct {
	Identity     NodeIdentity
	Type         NodeType
	Status       NodeStatus
	Endpoints    []string
	Version      string
	Capabilities []string
	Region       string
	RegisteredAt time.Time
	LastSeen     time.Time
}

// NodeMetrics representa as métricas de saúde de um nó
type NodeMetrics struct {
	CPUUsage               float32
	MemoryUsage            float32
	ActiveConnections      int32
	AvailableBandwidthMbps float32
	LatencyMs              int32
}

// IsRelay verifica se o nó é um relay server
func (n *Node) IsRelay() bool {
	return n.Type == NodeTypeRelay
}

// IsDesktop verifica se o nó é um cliente desktop
func (n *Node) IsDesktop() bool {
	return n.Type == NodeTypeDesktop
}

// HasCapability verifica se o nó tem uma capacidade específica
func (n *Node) HasCapability(cap string) bool {
	for _, c := range n.Capabilities {
		if c == cap {
			return true
		}
	}
	return false
}

// NodeRegistration representa os dados para registro de um nó
type NodeRegistration struct {
	Identity     NodeIdentity
	Type         NodeType
	Endpoints    []string
	Version      string
	Capabilities []string
	Region       string
}
