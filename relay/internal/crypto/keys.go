package crypto

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
)

// KeyPair representa um par de chaves Ed25519
type KeyPair struct {
	PrivateKey ed25519.PrivateKey
	PublicKey  ed25519.PublicKey
	NodeID     string
}

// LoadOrGenerateKeyPair carrega um par de chaves do arquivo ou gera um novo
func LoadOrGenerateKeyPair(keyPath string) (*KeyPair, error) {
	// Tenta carregar chave existente
	if data, err := os.ReadFile(keyPath); err == nil {
		if len(data) == ed25519.PrivateKeySize {
			return newKeyPairFromPrivate(data), nil
		}
	}

	// Gera novo par de chaves
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("failed to generate key pair: %w", err)
	}

	// Salva a chave privada
	if err := os.WriteFile(keyPath, priv, 0600); err != nil {
		return nil, fmt.Errorf("failed to save private key: %w", err)
	}

	return newKeyPairFromPrivate(priv), nil
}

// newKeyPairFromPrivate cria um KeyPair a partir da chave privada
func newKeyPairFromPrivate(priv ed25519.PrivateKey) *KeyPair {
	pub := priv.Public().(ed25519.PublicKey)
	nodeID := deriveNodeID(pub)

	return &KeyPair{
		PrivateKey: priv,
		PublicKey:  pub,
		NodeID:     nodeID,
	}
}

// deriveNodeID deriva o NodeID a partir da chave pública
func deriveNodeID(pub ed25519.PublicKey) string {
	hash := sha256.Sum256(pub)
	return hex.EncodeToString(hash[:16])
}

// Sign assina uma mensagem com a chave privada
func (kp *KeyPair) Sign(message []byte) []byte {
	return ed25519.Sign(kp.PrivateKey, message)
}

// SignNodeID assina o próprio NodeID (usado no registro)
func (kp *KeyPair) SignNodeID() []byte {
	return kp.Sign([]byte(kp.NodeID))
}
