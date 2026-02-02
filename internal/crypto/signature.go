package crypto

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"errors"
)

var (
	ErrInvalidPublicKey = errors.New("invalid public key: must be 32 bytes")
	ErrInvalidSignature = errors.New("invalid signature: must be 64 bytes")
	ErrSignatureInvalid = errors.New("signature verification failed")
	ErrNotMasterKey     = errors.New("public key is not a master moderator key")
)

// Verifier fornece métodos para verificação de assinaturas Ed25519
type Verifier struct {
	masterKeys map[string]ed25519.PublicKey
}

// NewVerifier cria um novo verificador com as chaves mestres fornecidas
func NewVerifier(masterKeyHexList []string) (*Verifier, error) {
	v := &Verifier{
		masterKeys: make(map[string]ed25519.PublicKey),
	}

	for _, keyHex := range masterKeyHexList {
		if keyHex == "" {
			continue
		}
		keyBytes, err := hex.DecodeString(keyHex)
		if err != nil {
			return nil, err
		}
		if len(keyBytes) != ed25519.PublicKeySize {
			return nil, ErrInvalidPublicKey
		}
		pubKey := ed25519.PublicKey(keyBytes)
		keyID := DeriveNodeID(pubKey)
		v.masterKeys[keyID] = pubKey
	}

	return v, nil
}

// VerifySignature verifica uma assinatura Ed25519
func VerifySignature(publicKey []byte, message []byte, signature []byte) error {
	if len(publicKey) != ed25519.PublicKeySize {
		return ErrInvalidPublicKey
	}
	if len(signature) != ed25519.SignatureSize {
		return ErrInvalidSignature
	}

	pubKey := ed25519.PublicKey(publicKey)
	if !ed25519.Verify(pubKey, message, signature) {
		return ErrSignatureInvalid
	}

	return nil
}

// VerifyNodeIdentity verifica se a assinatura do node_id é válida
func VerifyNodeIdentity(nodeID string, publicKey []byte, signature []byte) error {
	// Verifica se o nodeID foi derivado corretamente da chave pública
	expectedID := DeriveNodeID(publicKey)
	if nodeID != expectedID {
		return errors.New("node_id does not match public key")
	}

	// Verifica a assinatura do nodeID
	return VerifySignature(publicKey, []byte(nodeID), signature)
}

// VerifyMasterSignature verifica se uma assinatura foi feita por uma chave mestre
func (v *Verifier) VerifyMasterSignature(publicKey []byte, message []byte, signature []byte) error {
	if len(publicKey) != ed25519.PublicKeySize {
		return ErrInvalidPublicKey
	}

	keyID := DeriveNodeID(publicKey)
	masterKey, exists := v.masterKeys[keyID]
	if !exists {
		return ErrNotMasterKey
	}

	// Verifica se a chave fornecida corresponde à chave mestre
	for i := range masterKey {
		if masterKey[i] != publicKey[i] {
			return ErrNotMasterKey
		}
	}

	return VerifySignature(publicKey, message, signature)
}

// IsMasterKey verifica se uma chave pública é uma chave mestre
func (v *Verifier) IsMasterKey(publicKey []byte) bool {
	if len(publicKey) != ed25519.PublicKeySize {
		return false
	}
	keyID := DeriveNodeID(publicKey)
	_, exists := v.masterKeys[keyID]
	return exists
}

// DeriveNodeID deriva o ID do nó a partir da chave pública
// Formato: primeiros 16 bytes do SHA256 da chave pública, hex-encoded
func DeriveNodeID(publicKey []byte) string {
	hash := sha256.Sum256(publicKey)
	return hex.EncodeToString(hash[:16])
}

// Sign assina uma mensagem com uma chave privada Ed25519
func Sign(privateKey []byte, message []byte) ([]byte, error) {
	if len(privateKey) != ed25519.PrivateKeySize {
		return nil, errors.New("invalid private key: must be 64 bytes")
	}
	privKey := ed25519.PrivateKey(privateKey)
	signature := ed25519.Sign(privKey, message)
	return signature, nil
}
