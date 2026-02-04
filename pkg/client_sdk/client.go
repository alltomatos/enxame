package client_sdk

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/goautomatik/core-server/pkg/crypto/e2e"
	pbv1 "github.com/goautomatik/core-server/pkg/pb/v1"
	"github.com/goautomatik/core-server/pkg/relay"
	"github.com/goautomatik/core-server/pkg/storage"
)

const (
	defaultCoreAddr = "localhost:50051"

	// Protocol Constants
	MsgTypeKeyExchangeInit = "key_exchange_init"
	MsgTypeKeyExchangeAck  = "key_exchange_ack"
	MsgTypeChat            = "chat"
	MsgTypeJoinRequest     = "channel_join_request"
	MsgTypeKeyResponse     = "channel_key_response"
	MsgTypeSyncRequest     = "channel_sync_request"
	MsgTypeSyncResponse    = "channel_sync_response" // Grid Node -> New Member (Direct)
	MsgTypeFileManifest    = "file_manifest"
	MsgTypeChunkRequest    = "file_chunk_request"
	MsgTypeChunkResponse   = "file_chunk_response"
	MsgTypeFile            = "file"
	MsgTypeWikiUpdate      = "wiki_update"
	MsgTypeMetadataUpdate  = "metadata_update"
	MsgTypeTagCreated      = "tag_created"
	MsgTypeTagAdded        = "tag_added"
)

// Events
type MessageEvent struct {
	ID        string
	SenderID  string
	TargetID  string // Channel or NodeID
	Content   string
	Timestamp time.Time
	IsChannel bool
}

type EventCallback func(event MessageEvent)

type AuthCallback func(user *pbv1.User)

type GridCallback func(status string, jobID string)

type Member struct {
	NodeID   string
	Role     string
	IsOnline bool
}

// Helper to generate a consistent DM Channel ID based on two NodeIDs
func (c *EnxameClient) GetDMChannelID(targetID string) string {
	// Sort IDs to ensure consistency regardless of who started it
	ids := []string{c.nodeID, targetID}
	if c.nodeID > targetID {
		ids[0], ids[1] = targetID, c.nodeID
	}
	// Hash combined IDs
	hash := sha256.Sum256([]byte(ids[0] + ":" + ids[1]))
	return "dm_" + hex.EncodeToString(hash[:16])
}

type FileManifest struct {
	FileID   string      `json:"file_id"`
	Name     string      `json:"name"`
	Size     int64       `json:"size"`
	Chunks   []ChunkMeta `json:"chunks"`
	SenderID string      `json:"sender_id"`
}

type ChunkMeta struct {
	Index int    `json:"index"`
	Hash  string `json:"hash"` // SHA-256
}

type ChunkData struct {
	FileID string `json:"file_id"`
	Index  int    `json:"index"`
	Data   []byte `json:"data"`
}

type E2EPayload struct {
	Type    string `json:"type"`
	Content []byte `json:"content,omitempty"`
	IV      []byte `json:"iv,omitempty"`
}

type SessionE2E struct {
	SharedKey []byte
	CreatedAt time.Time
}

type EnxameClient struct {
	nodeID     string
	privateKey ed25519.PrivateKey
	publicKey  ed25519.PublicKey

	knownCores []string // Lista de core servers conhecidos (failover)
	coreClient pbv1.NetworkServiceClient
	coreConn   *grpc.ClientConn

	channelServiceClient pbv1.ChannelServiceClient
	relayClient          *relay.RelayClient
	relaysList           []*pbv1.NodeInfo

	storage *storage.Storage

	e2eSessions    map[string]*SessionE2E
	e2ePendingKeys map[string]*e2e.KeyPair

	currentChannel string
	channelKeys    map[string][]byte

	storageDir  string
	manifests   map[string]FileManifest
	pendingFile string

	mu sync.Mutex

	ctx          context.Context
	cancel       context.CancelFunc
	onMessage    EventCallback
	onGridStatus GridCallback
	gridJobs     int
	adminClient  pbv1.AdminServiceClient
	updateClient pbv1.UpdateServiceClient
	profile      string
	authToken    string
	vaultToken   string // Memória apenas
}

func NewEnxameClient(profile string) (*EnxameClient, error) {
	if profile == "" {
		profile = "default"
	}

	idFile := fmt.Sprintf("identity_%s.key", profile)
	if profile == "default" {
		idFile = "identity.key"
	}

	privKey, pubKey, nodeID, err := loadOrGenerateIdentity(idFile)
	if err != nil {
		return nil, fmt.Errorf("failed to load identity: %w", err)
	}

	dbFile := fmt.Sprintf("enxame_%s.db", profile)
	store, err := storage.NewStorage(dbFile)
	if err != nil {
		return nil, fmt.Errorf("failed to init storage: %w", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	relayClient := relay.NewRelayClient(nodeID, pubKey)

	c := &EnxameClient{
		nodeID:         nodeID,
		privateKey:     privKey,
		publicKey:      pubKey,
		knownCores:     []string{defaultCoreAddr}, // Seed inicial
		ctx:            ctx,
		cancel:         cancel,
		relayClient:    relayClient,
		storage:        store,
		e2eSessions:    make(map[string]*SessionE2E),
		e2ePendingKeys: make(map[string]*e2e.KeyPair),
		channelKeys:    make(map[string][]byte),
		manifests:      make(map[string]FileManifest),
		profile:        profile,
	}

	// Carregar cores persistidos do SQLite
	if savedCores, err := store.GetConfig("known_cores"); err == nil && savedCores != "" {
		c.knownCores = strings.Split(savedCores, ",")
	}

	c.storageDir = filepath.Join(filepath.Dir(c.storage.DBPath()), "file_transfer")
	os.MkdirAll(c.storageDir, 0755)
	os.MkdirAll(filepath.Join(c.storageDir, "chunks"), 0755)
	os.MkdirAll(filepath.Join(c.storageDir, "downloads"), 0755)

	// Carregar token de auth se existir no banco
	savedToken, _ := store.GetConfig("auth_token")
	if savedToken != "" {
		c.authToken = savedToken
	}

	return c, nil
}

func (c *EnxameClient) SetMessageCallback(cb EventCallback) {
	c.onMessage = cb
}

func (c *EnxameClient) SetGridCallback(cb GridCallback) {
	c.onGridStatus = cb
}

func (c *EnxameClient) GetNodeID() string {
	return c.nodeID
}

func (c *EnxameClient) GetIdentityPath() string {
	if c.profile == "default" {
		return "identity.key"
	}
	return fmt.Sprintf("identity_%s.key", c.profile)
}

func (c *EnxameClient) GetGridStats() map[string]int {
	return map[string]int{
		"jobs_processed": c.gridJobs,
		"uptime_seconds": 0, // Placeholder
	}
}

func (c *EnxameClient) Initialize() error {
	if err := c.connectToCore(); err != nil {
		return err
	}
	if err := c.register(); err != nil {
		return err
	}
	if err := c.updateRelaysAndConnect(); err != nil {
		return err
	}
	go c.bootstrapNetwork()
	go c.startGridWorker()
	return nil
}

func (c *EnxameClient) connectToCore() error {
	var lastErr error
	for _, addr := range c.knownCores {
		ctx, cancel := context.WithTimeout(c.ctx, 5*time.Second)
		conn, err := grpc.DialContext(ctx, addr, grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithBlock())
		cancel()
		if err != nil {
			lastErr = err
			log.Printf("[SDK] Failed to connect to core %s: %v", addr, err)
			continue
		}
		c.coreConn = conn
		c.coreClient = pbv1.NewNetworkServiceClient(conn)
		c.channelServiceClient = pbv1.NewChannelServiceClient(conn)
		c.adminClient = pbv1.NewAdminServiceClient(conn)
		c.updateClient = pbv1.NewUpdateServiceClient(conn)
		log.Printf("[SDK] Connected to core: %s", addr)

		// Auto-discovery: atualizar lista de cores do cluster
		go c.discoverClusterNodes()
		return nil
	}
	return fmt.Errorf("failed to connect to any core server: %w", lastErr)
}

// discoverClusterNodes consulta o core conectado para obter a lista de peers ativos
func (c *EnxameClient) discoverClusterNodes() {
	if c.coreConn == nil {
		return
	}

	clusterClient := pbv1.NewClusterServiceClient(c.coreConn)
	ctx, cancel := context.WithTimeout(c.ctx, 10*time.Second)
	defer cancel()

	resp, err := clusterClient.GetClusterNodes(ctx, &pbv1.GetClusterNodesRequest{})
	if err != nil {
		log.Printf("[SDK] Failed to discover cluster nodes: %v", err)
		return
	}

	if resp == nil || len(resp.Nodes) == 0 {
		return
	}

	// Atualizar lista de cores conhecidos
	c.mu.Lock()
	newCores := make([]string, 0, len(resp.Nodes))
	for _, node := range resp.Nodes {
		newCores = append(newCores, node.Address)
	}
	c.knownCores = newCores
	c.mu.Unlock()

	// Persistir no SQLite
	if err := c.storage.SetConfig("known_cores", strings.Join(newCores, ",")); err != nil {
		log.Printf("[SDK] Failed to persist known cores: %v", err)
	} else {
		log.Printf("[SDK] Discovered %d cluster nodes, saved to local storage", len(newCores))
	}
}

func (c *EnxameClient) register() error {
	signature := ed25519.Sign(c.privateKey, []byte(c.nodeID))
	req := &pbv1.RegisterNodeRequest{
		Identity: &pbv1.NodeIdentity{
			NodeId:    c.nodeID,
			PublicKey: c.publicKey,
			Signature: signature,
		},
		Type:         pbv1.NodeType_NODE_TYPE_DESKTOP,
		Endpoints:    []string{"localhost:0"},
		Version:      "1.2.0-sdk",
		Capabilities: []string{"desktop", "e2ee", "channels"},
		Region:       "default",
	}
	resp, err := c.coreClient.RegisterNode(c.ctx, req)
	if err != nil {
		return err
	}
	if !resp.Success {
		return fmt.Errorf("%s", resp.Message)
	}
	return nil
}

func (c *EnxameClient) updateRelaysAndConnect() error {
	resp, err := c.coreClient.GetActiveRelays(c.ctx, &pbv1.GetActiveRelaysRequest{Limit: 5})
	if err != nil {
		return err
	}
	c.relaysList = resp.Relays
	go c.connectToBestRelay()
	return nil
}

func (c *EnxameClient) connectToBestRelay() {
	for _, node := range c.relaysList {
		if len(node.Endpoints) == 0 {
			continue
		}
		endpoint := node.Endpoints[0]
		if err := c.relayClient.Connect(c.ctx, endpoint); err != nil {
			log.Printf("Failed to connect to %s: %v", endpoint, err)
			continue
		}
		// Re-subscribe if needed
		if c.currentChannel != "" {
			c.SubscribeToRelayChannel(c.currentChannel)
		}
		if err := c.relayClient.ReceiveLoop(c.handleMessage); err != nil {
			log.Printf("Relay connection lost: %v", err)
		}
		time.Sleep(1 * time.Second)
	}
}

func (c *EnxameClient) BootstrapNetwork() {
	c.bootstrapNetwork()
}

func (c *EnxameClient) bootstrapNetwork() {
	time.Sleep(2 * time.Second)
	for _, node := range c.relaysList {
		if node.Identity.NodeId == c.nodeID {
			continue
		}
		c.initiateHandshake(node.Identity.NodeId)
	}
}

func (c *EnxameClient) initiateHandshake(targetID string) {
	myPair, _ := e2e.GenerateKeyExchangePair()
	c.mu.Lock()
	c.e2ePendingKeys[targetID] = myPair
	c.mu.Unlock()
	payload := E2EPayload{Type: MsgTypeKeyExchangeInit, Content: myPair.Public.Bytes()}
	c.sendPayload(targetID, payload)
}

func (c *EnxameClient) handleMessage(envelope *pbv1.Envelope) {
	var payload E2EPayload
	if err := json.Unmarshal(envelope.EncryptedPayload, &payload); err != nil {
		return
	}

	switch payload.Type {
	case MsgTypeKeyExchangeInit:
		c.handleKeyExchangeInit(envelope.SenderNodeId, payload.Content)
	case MsgTypeKeyExchangeAck:
		c.handleKeyExchangeAck(envelope.SenderNodeId, payload.Content)
	case MsgTypeChat:
		c.handleChatMessage(envelope.SenderNodeId, envelope.TargetNodeId, payload.Content, envelope)
	case MsgTypeJoinRequest:
		c.handleChannelJoinRequest(envelope.SenderNodeId, payload.Content)
	case MsgTypeKeyResponse:
		c.handleChannelKeyResponse(envelope.SenderNodeId, payload.Content)
	case MsgTypeSyncRequest:
		c.handleSyncRequest(envelope.SenderNodeId, payload.Content)
	case MsgTypeSyncResponse:
		c.handleSyncResponse(envelope.SenderNodeId, payload.Content)
	case MsgTypeFileManifest:
		c.handleFileManifest(payload.Content)
	case MsgTypeChunkRequest:
		c.handleChunkRequest(envelope.SenderNodeId, payload.Content)
	case MsgTypeChunkResponse:
		c.handleChunkResponse(payload.Content)
	case MsgTypeFile:
		c.handleFilePayload(envelope.SenderNodeId, envelope.TargetNodeId, payload.Content, envelope)
	case MsgTypeWikiUpdate:
		c.handleWikiUpdate(envelope.SenderNodeId, envelope.TargetNodeId, payload.Content)
	case MsgTypeMetadataUpdate:
		c.handleMetadataUpdate(envelope.SenderNodeId, envelope.TargetNodeId, payload.Content)
	case MsgTypeTagCreated:
		c.handleTagCreated(envelope.SenderNodeId, envelope.TargetNodeId, payload.Content)
	case MsgTypeTagAdded:
		c.handleTagAdded(envelope.SenderNodeId, envelope.TargetNodeId, payload.Content)
	}
}

func (c *EnxameClient) handleWikiUpdate(senderID, targetID string, ciphertext []byte) {
	// Decrypt
	plaintext, err := c.decryptPayload(senderID, targetID, ciphertext)
	if err != nil {
		return
	}

	var page storage.WikiPage
	if err := json.Unmarshal(plaintext, &page); err != nil {
		return
	}
	c.storage.SaveWikiPage(page)
	// Notify UI
	if c.onMessage != nil {
		c.onMessage(MessageEvent{
			ID:        fmt.Sprintf("wiki-%s", page.ID),
			SenderID:  senderID,
			TargetID:  targetID,
			Content:   "[Wiki Update]",
			Timestamp: page.UpdatedAt,
			IsChannel: strings.HasPrefix(targetID, "#"),
		})
	}
}

func (c *EnxameClient) handleMetadataUpdate(senderID, targetID string, ciphertext []byte) {
	// Decrypt
	plaintext, err := c.decryptPayload(senderID, targetID, ciphertext)
	if err != nil {
		return
	}

	var config storage.ChannelConfig
	if err := json.Unmarshal(plaintext, &config); err != nil {
		return
	}
	c.storage.SetChannelConfig(config.ChannelID, config.Key, config.Value)
	// Process special keys
	if config.Key == "avatar" {
		c.storage.UpdateChannelAvatar(config.ChannelID, config.Value)
	}
	// Notify UI (Sidebar updates?)
}

// Helper for decryption (extracted for reuse)
func (c *EnxameClient) decryptPayload(senderID, targetID string, ciphertext []byte) ([]byte, error) {
	var key []byte
	isChannel := strings.HasPrefix(targetID, "#")
	if isChannel {
		k, ok := c.channelKeys[targetID]
		if !ok {
			hexKey, err := c.storage.GetChannelKey(targetID)
			if err == nil {
				k, _ = hex.DecodeString(hexKey)
				c.channelKeys[targetID] = k
			} else {
				return nil, fmt.Errorf("channel key missing")
			}
		}
		key = k
	} else {
		c.mu.Lock()
		session, exists := c.e2eSessions[senderID]
		c.mu.Unlock()
		if exists {
			key = session.SharedKey
		} else {
			return nil, fmt.Errorf("session missing")
		}
	}
	return e2e.Decrypt(key, ciphertext)
}

func (c *EnxameClient) handleFilePayload(senderID, targetID string, ciphertext []byte, envelope *pbv1.Envelope) {
	// Decrypt Logic (Duplicated from handleChatMessage for MVP simplicity)
	var key []byte
	isChannel := strings.HasPrefix(targetID, "#")
	if isChannel {
		k, ok := c.channelKeys[targetID]
		if !ok {
			hexKey, err := c.storage.GetChannelKey(targetID)
			if err == nil {
				key, _ = hex.DecodeString(hexKey)
			}
		} else {
			key = k
		}
	} else {
		c.mu.Lock()
		session, exists := c.e2eSessions[senderID]
		c.mu.Unlock()
		if exists {
			key = session.SharedKey
		}
	}

	plaintext, err := e2e.Decrypt(key, ciphertext)
	if err != nil {
		return
	}

	var manifest FileManifest
	if err := json.Unmarshal(plaintext, &manifest); err != nil {
		return
	}

	// Store manifest in memory and potentially auto-download valid chunks via grid (later)
	c.manifests[manifest.FileID] = manifest

	// Determine storage target
	storageTarget := targetID
	if !isChannel {
		dmID := c.GetDMChannelID(senderID)
		storageTarget = dmID
	}

	// Store special message format
	fileMsg := fmt.Sprintf("[FILE:%s:%s:%d]", manifest.FileID, manifest.Name, manifest.Size)
	c.storage.StoreMessage(envelope.MessageId, senderID, storageTarget, fileMsg)

	// Notify UI
	if c.onMessage != nil {
		c.onMessage(MessageEvent{
			ID:        envelope.MessageId,
			SenderID:  senderID,
			TargetID:  storageTarget,
			Content:   fileMsg,
			Timestamp: envelope.Timestamp.AsTime(),
			IsChannel: isChannel,
		})
	}

}

func (c *EnxameClient) handleKeyExchangeInit(senderID string, peerPubKeyBytes []byte) {
	myPair, _ := e2e.GenerateKeyExchangePair()
	sharedSecret, _ := e2e.DeriveSharedSecret(myPair.Private, peerPubKeyBytes)
	c.mu.Lock()
	c.e2eSessions[senderID] = &SessionE2E{SharedKey: sharedSecret, CreatedAt: time.Now()}
	c.mu.Unlock()
	ack := E2EPayload{Type: MsgTypeKeyExchangeAck, Content: myPair.Public.Bytes()}
	c.sendPayload(senderID, ack)
	log.Printf("Session established with %s", senderID)
}

func (c *EnxameClient) handleKeyExchangeAck(senderID string, peerPubKeyBytes []byte) {
	c.mu.Lock()
	defer c.mu.Unlock()
	myPair, exists := c.e2ePendingKeys[senderID]
	if !exists {
		return
	}
	sharedSecret, _ := e2e.DeriveSharedSecret(myPair.Private, peerPubKeyBytes)
	c.e2eSessions[senderID] = &SessionE2E{SharedKey: sharedSecret, CreatedAt: time.Now()}
	delete(c.e2ePendingKeys, senderID)
	log.Printf("Handshake complete with %s", senderID)
}

func (c *EnxameClient) handleChatMessage(senderID, targetID string, ciphertext []byte, envelope *pbv1.Envelope) {
	var key []byte
	isChannel := strings.HasPrefix(targetID, "#")

	if isChannel {
		k, ok := c.channelKeys[targetID]
		if !ok {
			hexKey, err := c.storage.GetChannelKey(targetID)
			if err == nil && hexKey != "" {
				k, _ = hex.DecodeString(hexKey)
				c.channelKeys[targetID] = k
			} else {
				return
			}
		}
		key = k
	} else {
		c.mu.Lock()
		session, exists := c.e2eSessions[senderID]
		c.mu.Unlock()
		if !exists {
			return
		}
		key = session.SharedKey
	}

	plaintext, err := e2e.Decrypt(key, ciphertext)
	if err != nil {
		return
	}

	// Determine storage Target ID
	storageTarget := targetID
	if !isChannel {
		// It's a DM. We need to store it under the DM Channel ID.
		// Since I am receiving, the other party is 'senderID'.
		dmID := c.GetDMChannelID(senderID)
		storageTarget = dmID
		c.storage.UpsertDMSession(dmID, senderID)
	}

	c.storage.StoreMessage(envelope.MessageId, senderID, storageTarget, string(plaintext))

	// Track member status
	if isChannel {
		c.storage.UpsertMember(targetID, senderID, "MEMBER") // Default role
	} else {
		// For DM, maybe track last seen too?
		// c.storage.UpsertMember(storageTarget, senderID, "USER") // Optional
	}

	// NOTIFY UI
	if c.onMessage != nil {
		c.onMessage(MessageEvent{
			ID:        envelope.MessageId,
			SenderID:  senderID,
			TargetID:  storageTarget, // Pass the DM ID to UI so it opens/highlights correct chat
			Content:   string(plaintext),
			Timestamp: envelope.Timestamp.AsTime(),
			IsChannel: isChannel,
		})
	}
}

func (c *EnxameClient) SendFile(targetID, filePath string) error {
	f, err := os.Open(filePath)
	if err != nil {
		return err
	}
	defer f.Close()

	stat, _ := f.Stat()
	if stat.Size() > 10*1024*1024 { // 10MB Limit
		return fmt.Errorf("file too large (max 10MB)")
	}

	fileName := filepath.Base(filePath)
	fileID := fmt.Sprintf("file_%d", time.Now().UnixNano())

	// prepare chunks
	const chunkSize = 512 * 1024
	buf := make([]byte, chunkSize)
	var chunks []ChunkMeta
	var chunkIndex int

	for {
		n, err := f.Read(buf)
		if err != nil && err != io.EOF {
			return err
		}
		if n == 0 {
			break
		}

		// Hash chunk
		hash := sha256.Sum256(buf[:n])
		hashHex := hex.EncodeToString(hash[:])
		chunks = append(chunks, ChunkMeta{Index: chunkIndex, Hash: hashHex})

		// Save chunk locally (for seeding)
		chunkPath := filepath.Join(c.storageDir, "chunks", fmt.Sprintf("%s_%d.dat", fileID, chunkIndex))
		os.WriteFile(chunkPath, buf[:n], 0644)

		chunkIndex++
	}

	manifest := FileManifest{
		FileID:   fileID,
		Name:     fileName,
		Size:     stat.Size(),
		Chunks:   chunks,
		SenderID: c.nodeID,
	}

	// Determine storage Target ID
	isChannel := strings.HasPrefix(targetID, "#")
	storageTarget := targetID
	if !isChannel {
		dmID := c.GetDMChannelID(targetID)
		storageTarget = dmID
		c.storage.UpsertDMSession(dmID, targetID)
	}

	// Encrypt & Send Manifest
	manifestData, _ := json.Marshal(manifest)

	// Use SendMessage logic for keys but send MsgTypeFile
	var key []byte
	if isChannel {
		k, ok := c.channelKeys[targetID]
		if !ok {
			// simplified: assume exists or fetch (reusing logic from SendMessage best effort)
			hexKey, err := c.storage.GetChannelKey(targetID)
			if err == nil {
				key, _ = hex.DecodeString(hexKey)
			}
		} else {
			key = k
		}
	} else {
		c.mu.Lock()
		session, exists := c.e2eSessions[targetID]
		c.mu.Unlock()
		if exists {
			key = session.SharedKey
		}
	}

	if len(key) == 0 {
		return fmt.Errorf("encryption key not found")
	}

	ciphertext, _ := e2e.Encrypt(key, manifestData)
	payload := E2EPayload{Type: MsgTypeFile, Content: ciphertext}
	c.sendPayload(targetID, payload)

	// Save file manifest locally as a "message" so it shows in chat
	fileMsg := fmt.Sprintf("[FILE:%s:%s:%d]", fileID, fileName, stat.Size())
	c.storage.StoreMessage(fmt.Sprintf("msg-%d", time.Now().UnixNano()), c.nodeID, storageTarget, fileMsg)
	c.manifests[fileID] = manifest // Store in memory for seeding logic

	return nil
}

// SaveFileToDisk reassembles a file from local chunks
func (c *EnxameClient) SaveFileToDisk(fileID string, destPath string) error {
	manifest, ok := c.manifests[fileID]
	if !ok {
		return fmt.Errorf("manifest not found for file %s", fileID)
	}

	out, err := os.Create(destPath)
	if err != nil {
		return err
	}
	defer out.Close()

	for _, chunk := range manifest.Chunks {
		chunkPath := filepath.Join(c.storageDir, "chunks", fmt.Sprintf("%s_%d.dat", fileID, chunk.Index))
		data, err := os.ReadFile(chunkPath)
		if err != nil {
			return fmt.Errorf("missing chunk %d: %v", chunk.Index, err)
		}

		// Verify Hash (Optional but recommended)
		hash := sha256.Sum256(data)
		if hex.EncodeToString(hash[:]) != chunk.Hash {
			return fmt.Errorf("chunk %d hash mismatch", chunk.Index)
		}

		if _, err := out.Write(data); err != nil {
			return err
		}
	}
	return nil
}

func (c *EnxameClient) SendMessage(targetID, message string) error {
	if !c.relayClient.IsConnected() {
		return fmt.Errorf("not connected to relay")
	}

	var key []byte
	isChannel := strings.HasPrefix(targetID, "#")

	if isChannel {
		k, ok := c.channelKeys[targetID]
		if !ok {
			hexKey, err := c.storage.GetChannelKey(targetID)
			if err == nil && hexKey != "" {
				k, _ = hex.DecodeString(hexKey)
				c.channelKeys[targetID] = k
			}
			key = k
		}
		if len(key) == 0 {
			return fmt.Errorf("no key for channel %s", targetID)
		}
	} else {
		c.mu.Lock()
		session, exists := c.e2eSessions[targetID]
		c.mu.Unlock()
		if !exists {
			c.initiateHandshake(targetID)
			return fmt.Errorf("initiating handshake with %s, try again", targetID)
		}
		key = session.SharedKey
	}

	// Encrypt
	ciphertext, err := e2e.Encrypt(key, []byte(message))
	if err != nil {
		return err
	}

	payload := E2EPayload{Type: MsgTypeChat, Content: ciphertext}
	msgID := c.sendPayload(targetID, payload)

	// Determine storage Target ID
	storageTarget := targetID
	if !isChannel {
		dmID := c.GetDMChannelID(targetID)
		storageTarget = dmID
		c.storage.UpsertDMSession(dmID, targetID)
	}

	c.storage.StoreMessage(msgID, c.nodeID, storageTarget, message)
	return nil
}

func (c *EnxameClient) sendPayload(targetID string, payload E2EPayload) string {
	data, _ := json.Marshal(payload)
	msgID := fmt.Sprintf("msg-%d", time.Now().UnixNano())
	envelope := &pbv1.Envelope{
		MessageId:        msgID,
		SenderNodeId:     c.nodeID,
		TargetNodeId:     targetID,
		EncryptedPayload: data,
		SenderSignature:  ed25519.Sign(c.privateKey, data),
		Timestamp:        timestamppb.Now(),
		MessageType:      pbv1.MessageType_MESSAGE_TYPE_CHAT,
	}
	c.relayClient.Send(envelope)
	return msgID
}

// Channel & Governance Wrapper Methods

func (c *EnxameClient) CreateChannel(name string) (string, error) {
	name = strings.TrimPrefix(name, "#")
	res, err := c.channelServiceClient.CreateChannel(c.ctx, &pbv1.CreateChannelRequest{
		Name:        name,
		OwnerNodeId: c.nodeID,
	})
	if err != nil {
		return "", err
	}
	if !res.Success {
		return "", fmt.Errorf(res.Message)
	}

	channelID := res.ChannelId
	key := make([]byte, 32)
	rand.Read(key)
	keyHex := hex.EncodeToString(key)

	c.storage.SaveChannelKey(channelID, keyHex)
	c.channelKeys[channelID] = key

	c.SubscribeToRelayChannel(channelID)
	return channelID, nil
}

func (c *EnxameClient) JoinChannel(channelID string) error {
	if !strings.HasPrefix(channelID, "#") {
		channelID = "#" + channelID
	}
	c.currentChannel = channelID
	c.SubscribeToRelayChannel(channelID)

	// Check if we have key, if not request from owner (MVP simplified: broadcast request)
	// For now, assume key exists or handled by CLI logic.
	// To fully implement SDK, we should handle join request logic here.
	// Let's rely on the manual flow: If no key, user must use /join to trigger potential key fetch or fail.
	// But in SDK, JoinChannel simply subscribes locally and on relay.
	return nil
}

func (c *EnxameClient) SubscribeToRelayChannel(channelID string) {
	cmd := struct {
		Command string `json:"command"`
		Channel string `json:"channel"`
	}{
		Command: "subscribe",
		Channel: channelID,
	}
	payload, _ := json.Marshal(cmd)
	env := &pbv1.Envelope{
		MessageId:        fmt.Sprintf("ctrl-%d", time.Now().UnixNano()),
		SenderNodeId:     c.nodeID,
		TargetNodeId:     "RELAY",
		EncryptedPayload: payload,
		SenderSignature:  ed25519.Sign(c.privateKey, payload),
		Timestamp:        timestamppb.Now(),
		MessageType:      pbv1.MessageType_MESSAGE_TYPE_CONTROL,
	}
	c.relayClient.Send(env)
}

func (c *EnxameClient) GetHistory(target string) ([]storage.Message, error) {
	return c.storage.GetMessages(target, 50)
}

func (c *EnxameClient) GetJoinedChannels() ([]string, error) {
	return c.storage.GetAllChannels()
}

func (c *EnxameClient) GetRecentChats() ([]storage.ChatItem, error) {
	return c.storage.GetRecentChats()
}

func (c *EnxameClient) GetChannelMembers(channelID string) ([]Member, error) {
	storedMembers, err := c.storage.GetChannelMembers(channelID)
	if err != nil {
		return nil, err
	}

	var members []Member
	now := time.Now()
	for _, m := range storedMembers {
		// Simple logic: Seen in last 5 minutes = Online
		isOnline := now.Sub(m.LastSeen) < 5*time.Minute
		members = append(members, Member{
			NodeID:   m.NodeID,
			Role:     m.Role,
			IsOnline: isOnline,
		})
	}
	return members, nil
}

func (c *EnxameClient) Close() {
	c.cancel()
	if c.storage != nil {
		c.storage.Close()
	}
	if c.coreConn != nil {
		c.coreConn.Close()
	}
	if c.relayClient != nil {
		c.relayClient.Close()
	}
}

// Internal Helpers (Persistence, Identity)
func loadOrGenerateIdentity(path string) (ed25519.PrivateKey, ed25519.PublicKey, string, error) {
	if data, err := os.ReadFile(path); err == nil && len(data) == ed25519.PrivateKeySize {
		priv := ed25519.PrivateKey(data)
		pub := priv.Public().(ed25519.PublicKey)
		nodeID := deriveNodeID(pub)
		return priv, pub, nodeID, nil
	}
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, nil, "", err
	}
	if err := os.WriteFile(path, priv, 0600); err != nil {
		return nil, nil, "", err
	}
	nodeID := deriveNodeID(pub)
	return priv, pub, nodeID, nil
}

func deriveNodeID(pub ed25519.PublicKey) string {
	hash := sha256.Sum256(pub)
	return hex.EncodeToString(hash[:16])
}

// --- Governance ---

func (c *EnxameClient) PromoteUser(target, role, channel string) error {
	roleEnum := pbv1.Role_MEMBER
	if role == "ADMIN" {
		roleEnum = pbv1.Role_ADMIN
	} else if role == "MODERATOR" {
		roleEnum = pbv1.Role_MODERATOR
	} else {
		return fmt.Errorf("invalid role")
	}

	ts := time.Now().UnixNano()
	payload := fmt.Sprintf("%s|%s|%s|%s|%d", target, roleEnum, pbv1.Scope_CHANNEL, channel, ts)
	sig := ed25519.Sign(c.privateKey, []byte(payload))

	resp, err := c.channelServiceClient.PromoteUser(c.ctx, &pbv1.PromoteRequest{
		TargetNodeId: target,
		Role:         roleEnum,
		Scope:        pbv1.Scope_CHANNEL,
		ChannelId:    channel,
		RequesterId:  c.nodeID,
		Timestamp:    ts,
		Signature:    sig,
	})
	if err != nil {
		return err
	}
	if !resp.Success {
		return fmt.Errorf(resp.Message)
	}
	return nil
}

func (c *EnxameClient) DemoteUser(target, channel string) error {
	ts := time.Now().UnixNano()
	payload := fmt.Sprintf("%s||%s|%s|%d", target, pbv1.Scope_CHANNEL, channel, ts)
	sig := ed25519.Sign(c.privateKey, []byte(payload))

	resp, err := c.channelServiceClient.DemoteUser(c.ctx, &pbv1.DemoteRequest{
		TargetNodeId: target,
		Scope:        pbv1.Scope_CHANNEL,
		ChannelId:    channel,
		RequesterId:  c.nodeID,
		Timestamp:    ts,
		Signature:    sig,
	})
	if err != nil {
		return err
	}
	if !resp.Success {
		return fmt.Errorf(resp.Message)
	}
	return nil
}

func (c *EnxameClient) KickUser(target, channel string) error {
	ts := time.Now().UnixNano()
	payload := fmt.Sprintf("%s||%s|%s|%d", target, pbv1.Scope_CHANNEL, channel, ts) // Role empty
	sig := ed25519.Sign(c.privateKey, []byte(payload))

	resp, err := c.channelServiceClient.KickUser(c.ctx, &pbv1.KickRequest{
		TargetNodeId: target,
		Scope:        pbv1.Scope_CHANNEL,
		ChannelId:    channel,
		RequesterId:  c.nodeID,
		Timestamp:    ts,
		Signature:    sig,
	})
	if err != nil {
		return err
	}
	if !resp.Success {
		return fmt.Errorf(resp.Message)
	}
	return nil
}

// --- Grid & File Handlers (Simplified for SDK) ---

func (c *EnxameClient) startGridWorker() {
	if c.coreConn == nil {
		return
	}
	gridClient := pbv1.NewGridServiceClient(c.coreConn)

	for {
		if c.ctx.Err() != nil {
			return
		}

		stream, err := gridClient.SubscribeToJobs(c.ctx, &pbv1.NodeIdentity{
			NodeId:    c.nodeID,
			PublicKey: c.publicKey,
		})
		if err != nil {
			time.Sleep(5 * time.Second)
			continue
		}

		for {
			job, err := stream.Recv()
			if err != nil {
				break
			}
			// Notify UI of Grid Job
			if c.onGridStatus != nil {
				c.onGridStatus("Syncing...", job.JobId)
			}
			// Simulate Process
			time.Sleep(1 * time.Second)
			gridClient.SubmitResult(context.Background(), &pbv1.SubmitJobResultRequest{
				JobId:    job.JobId,
				WorkerId: c.nodeID,
				Success:  true,
			})
			c.mu.Lock()
			c.gridJobs++
			c.mu.Unlock()
			if c.onGridStatus != nil {
				c.onGridStatus("Idle", "")
			}
		}
		time.Sleep(3 * time.Second)
	}
}

// Handlers for Channel logic (Join/Key/Sync/File) - Should be ported from Main
// For brevity, I'll include basic ones. Full port would separate into files.

func (c *EnxameClient) handleChannelJoinRequest(senderID string, channelIDBytes []byte) {
	channelID := string(channelIDBytes)
	c.mu.Lock()
	key, exists := c.channelKeys[channelID]
	c.mu.Unlock()

	if !exists {
		storedKeyHex, err := c.storage.GetChannelKey(channelID)
		if err == nil && storedKeyHex != "" {
			key, _ = hex.DecodeString(storedKeyHex)
			exists = true
		}
	}

	// Update member status
	c.storage.UpsertMember(channelID, senderID, "MEMBER")

	if exists {
		c.sendSecureMessage(senderID, MsgTypeKeyResponse, []byte(fmt.Sprintf("%s|%x", channelID, key)))
	}
}

func (c *EnxameClient) handleChannelKeyResponse(senderID string, content []byte) {
	parts := strings.SplitN(string(content), "|", 2)
	if len(parts) == 2 {
		channelID := parts[0]
		keyHex := parts[1]
		c.storage.SaveChannelKey(channelID, keyHex)
		keyBytes, _ := hex.DecodeString(keyHex)
		c.mu.Lock()
		c.channelKeys[channelID] = keyBytes
		c.mu.Unlock()
		c.SubscribeToRelayChannel(channelID)

		// Event or auto-join logic
		if c.onMessage != nil {
			c.onMessage(MessageEvent{
				Content:   fmt.Sprintf("Joined %s", channelID),
				IsChannel: true,
				TargetID:  channelID,
			})
		}
	}
}

func (c *EnxameClient) handleSyncRequest(requesterID string, channelIDBytes []byte) {
	// Grid logic
}

func (c *EnxameClient) handleSyncResponse(senderID string, content []byte) {
	// Sync logic
}

func (c *EnxameClient) handleFileManifest(content []byte) {
	// File logic
}

func (c *EnxameClient) handleChunkRequest(senderID string, content []byte) {
	// Simple MVP: Direct Response
	// Content = "fileID|chunkIndex"
	parts := strings.Split(string(content), "|")
	if len(parts) != 2 {
		return
	}
	fileID := parts[0]
	// index := parts[1] // string

	// Ideally validate permissions via manifest/channel
	// For MVP, just serve if we have it
	chunkPath := filepath.Join(c.storageDir, "chunks", fmt.Sprintf("%s_%s.dat", fileID, parts[1]))
	data, err := os.ReadFile(chunkPath)
	if err != nil {
		return // Chunk not found
	}

	// Send Response: "fileID|chunkIndex|data_base64" or just raw bytes if we had better protocol
	// Reuse E2E for simplicity, though overhead is high for files
	// Response: JSON { FileID, Index, Data }
	resp := struct {
		FileID string `json:"file_id"`
		Index  string `json:"index"`
		Data   []byte `json:"data"`
	}{
		FileID: fileID,
		Index:  parts[1],
		Data:   data,
	}

	respBytes, _ := json.Marshal(resp)
	c.sendSecureMessage(senderID, MsgTypeChunkResponse, respBytes)
}

func (c *EnxameClient) handleChunkResponse(content []byte) {
	// Receive chunk
	var resp struct {
		FileID string `json:"file_id"`
		Index  string `json:"index"`
		Data   []byte `json:"data"`
	}
	if err := json.Unmarshal(content, &resp); err != nil {
		return
	}

	// Validate against manifest?
	// Save to disk
	chunkPath := filepath.Join(c.storageDir, "chunks", fmt.Sprintf("%s_%s.dat", resp.FileID, resp.Index))
	os.WriteFile(chunkPath, resp.Data, 0644)

	// Notify UI or Download Manager?
	if c.onMessage != nil {
		// Maybe a special event or just log
	}
}

// ==========================================
// CHANNEL HUB METHODS (Wiki & Video)
// ==========================================

func (c *EnxameClient) SaveWikiPage(channelID, title, content string) error {
	id := fmt.Sprintf("wiki_%d", time.Now().UnixNano()) // Simple ID
	page := storage.WikiPage{
		ID:        id,
		ChannelID: channelID,
		Title:     title,
		Content:   content,
		UpdatedAt: time.Now(),
	}

	// 1. Save Local
	if err := c.storage.SaveWikiPage(page); err != nil {
		return err
	}

	// 2. Broadcast Update
	data, _ := json.Marshal(page)
	// Encrypt for Channel
	key, ok := c.channelKeys[channelID]
	if !ok {
		// Try fetch
		hexKey, err := c.storage.GetChannelKey(channelID)
		if err == nil && hexKey != "" {
			key, _ = hex.DecodeString(hexKey)
			c.channelKeys[channelID] = key
		} else {
			return fmt.Errorf("channel key not found")
		}
	}

	ciphertext, _ := e2e.Encrypt(key, data)
	payload := E2EPayload{Type: MsgTypeWikiUpdate, Content: ciphertext}
	c.sendPayload(channelID, payload)
	return nil
}

func (c *EnxameClient) GetWikiPages(channelID string) ([]storage.WikiPage, error) {
	return c.storage.GetWikiPages(channelID)
}

func (c *EnxameClient) SetChannelConfig(channelID, key, value string) error {
	// 1. Verify Role: Must be Owner or Admin
	myRole := "USER" // Default
	member, err := c.storage.GetMember(channelID, c.nodeID)
	if err == nil {
		myRole = member.Role
	}

	if myRole != "OWNER" && myRole != "ADMIN" {
		return fmt.Errorf("permission denied: only ADMIN or OWNER can set channel configuration")
	}

	// 2. Save Local
	if err := c.storage.SetChannelConfig(channelID, key, value); err != nil {
		return err
	}

	// 3. Broadcast Update
	cfg := storage.ChannelConfig{
		ChannelID: channelID,
		Key:       key,
		Value:     value,
	}
	data, _ := json.Marshal(cfg)

	keyBytes, ok := c.channelKeys[channelID]
	if !ok {
		// Try fetch
		hexKey, err := c.storage.GetChannelKey(channelID)
		if err == nil && hexKey != "" {
			keyBytes, _ = hex.DecodeString(hexKey)
			c.channelKeys[channelID] = keyBytes
		} else {
			return fmt.Errorf("channel key not found")
		}
	}

	ciphertext, _ := e2e.Encrypt(keyBytes, data)
	payload := E2EPayload{Type: MsgTypeMetadataUpdate, Content: ciphertext}
	c.sendPayload(channelID, payload)
	return nil
}

func (c *EnxameClient) GetChannelConfig(channelID, key string) (string, error) {
	return c.storage.GetChannelConfig(channelID, key)
}

func (c *EnxameClient) sendSecureMessage(targetID, msgType string, content []byte) {
	c.mu.Lock()
	session, exists := c.e2eSessions[targetID]
	c.mu.Unlock()

	if !exists {
		c.initiateHandshake(targetID)
		return
	}

	ciphertext, _ := e2e.Encrypt(session.SharedKey, content)
	payload := E2EPayload{Type: msgType, Content: ciphertext}
	c.sendPayload(targetID, payload)
}

func (c *EnxameClient) UpdateChannelAvatar(channelID, avatar string) error {
	// 1. Perm Check
	m, err := c.storage.GetMember(channelID, c.nodeID)
	if err != nil || (m.Role != "ADMIN" && m.Role != "OWNER") {
		return fmt.Errorf("permission denied: only ADMIN or OWNER can update avatar")
	}

	// 2. Sign Request
	ts := time.Now().UnixNano()
	payload := fmt.Sprintf("%s|%s|%d", channelID, avatar, ts)
	sig := ed25519.Sign(c.privateKey, []byte(payload))

	// 3. Call Core
	resp, err := c.channelServiceClient.UpdateChannelAvatar(c.ctx, &pbv1.UpdateChannelAvatarRequest{
		ChannelId:   channelID,
		Avatar:      avatar,
		RequesterId: c.nodeID,
		Timestamp:   ts,
		Signature:   sig,
	})
	if err != nil {
		return err
	}
	if !resp.Success {
		return fmt.Errorf(resp.Message)
	}

	// 4. Update Local
	return c.storage.UpdateChannelAvatar(channelID, avatar)
}

// ==========================================
// TAGS SYSTEM METHODS
// ==========================================

func (c *EnxameClient) CreateTag(channelID, name, color string) error {
	// 1. Permission Check
	myRole := "USER"
	if m, err := c.storage.GetMember(channelID, c.nodeID); err == nil {
		myRole = m.Role
	}
	if myRole != "ADMIN" && myRole != "OWNER" {
		return fmt.Errorf("permission denied: only ADMIN or OWNER can create tags")
	}

	// 2. Create Tag Object
	tag := storage.ChannelTag{
		ID:        fmt.Sprintf("tag-%d", time.Now().UnixNano()),
		ChannelID: channelID,
		Name:      name,
		Color:     color,
	}

	// 3. Save Local
	if err := c.storage.CreateTag(tag); err != nil {
		return err
	}

	// 4. Broadcast
	data, _ := json.Marshal(tag)
	key, err := c.getChannelKey(channelID)
	if err != nil {
		return err
	}

	ciphertext, _ := e2e.Encrypt(key, data)
	c.sendPayload(channelID, E2EPayload{Type: MsgTypeTagCreated, Content: ciphertext})

	return nil
}

func (c *EnxameClient) TagMessage(channelID, messageID, tagID string) error {
	// 1. Create Link
	mt := storage.MessageTag{
		MessageID: messageID,
		TagID:     tagID,
		TaggedBy:  c.nodeID,
	}

	// 2. Save Local
	if err := c.storage.AddMessageTag(mt); err != nil {
		return err
	}

	// 3. Broadcast
	data, _ := json.Marshal(mt)
	key, err := c.getChannelKey(channelID)
	if err != nil {
		return err
	}

	ciphertext, _ := e2e.Encrypt(key, data)
	c.sendPayload(channelID, E2EPayload{Type: MsgTypeTagAdded, Content: ciphertext})

	return nil
}

func (c *EnxameClient) GetTags(channelID string) ([]storage.ChannelTag, error) {
	return c.storage.GetTags(channelID)
}

func (c *EnxameClient) GetMessageTags(channelID string) (map[string][]storage.MessageTag, error) {
	return c.storage.GetMessageTags(channelID)
}

func (c *EnxameClient) handleTagCreated(senderID, targetID string, ciphertext []byte) {
	plaintext, err := c.decryptPayload(senderID, targetID, ciphertext)
	if err != nil {
		return
	}
	var t storage.ChannelTag
	if err := json.Unmarshal(plaintext, &t); err != nil {
		return
	}
	c.storage.CreateTag(t)
	if c.onMessage != nil {
		c.onMessage(MessageEvent{ID: fmt.Sprintf("tag-%s", t.ID), SenderID: senderID, TargetID: targetID, Content: "[Tag Created: " + t.Name + "]", Timestamp: time.Now(), IsChannel: true})
	}
}

func (c *EnxameClient) handleTagAdded(senderID, targetID string, ciphertext []byte) {
	plaintext, err := c.decryptPayload(senderID, targetID, ciphertext)
	if err != nil {
		return
	}
	var mt storage.MessageTag
	if err := json.Unmarshal(plaintext, &mt); err != nil {
		return
	}
	c.storage.AddMessageTag(mt)
	if c.onMessage != nil {
		c.onMessage(MessageEvent{ID: fmt.Sprintf("tag-add-%s", mt.TagID), SenderID: senderID, TargetID: targetID, Content: "[Tag Added]", Timestamp: time.Now(), IsChannel: true})
	}
}

func (c *EnxameClient) getChannelKey(channelID string) ([]byte, error) {
	if k, ok := c.channelKeys[channelID]; ok {
		return k, nil
	}
	hexKey, err := c.storage.GetChannelKey(channelID)
	if err != nil {
		return nil, err
	}
	k, err := hex.DecodeString(hexKey)
	if err == nil {
		c.channelKeys[channelID] = k
	}
	return k, err
}

// -- AUTH METHODS --

func (c *EnxameClient) withAuth(ctx context.Context) context.Context {
	md := metadata.Pairs("authorization", "Bearer "+c.authToken)
	if c.vaultToken != "" {
		md.Set("x-vault-token", c.vaultToken)
	}
	return metadata.NewOutgoingContext(ctx, md)
}

func (c *EnxameClient) Register(email, password, fullName, phone, nickname string) (*pbv1.RegisterResponse, error) {
	conn, err := c.getCoreConn()
	if err != nil {
		return nil, err
	}

	client := pbv1.NewAuthServiceClient(conn)
	resp, err := client.Register(context.Background(), &pbv1.RegisterRequest{
		Email:    email,
		Password: password,
		FullName: fullName,
		Phone:    phone,
		Nickname: nickname,
	})

	if err == nil && resp.Success {
		c.authToken = resp.Token
		c.storage.SetConfig("auth_token", resp.Token)
	}

	return resp, err
}

func (c *EnxameClient) Login(email, password string) (*pbv1.LoginResponse, error) {
	conn, err := c.getCoreConn()
	if err != nil {
		return nil, err
	}

	client := pbv1.NewAuthServiceClient(conn)
	resp, err := client.Login(context.Background(), &pbv1.LoginRequest{
		Email:    email,
		Password: password,
	})

	if err == nil && resp.Success {
		c.authToken = resp.Token
		c.storage.SetConfig("auth_token", resp.Token)
	}

	return resp, err
}

func (c *EnxameClient) GetMe() (*pbv1.GetMeResponse, error) {
	conn, err := c.getCoreConn()
	if err != nil {
		return nil, err
	}

	client := pbv1.NewAuthServiceClient(conn)
	return client.GetMe(c.withAuth(context.Background()), &pbv1.GetMeRequest{})
}

func (c *EnxameClient) Logout() {
	c.authToken = ""
	c.storage.SetConfig("auth_token", "")
}

// --- Admin Services ---

func (c *EnxameClient) GetNetworkStats() (*pbv1.GetNetworkStatsResponse, error) {
	return c.adminClient.GetNetworkStats(c.withAuth(context.Background()), &pbv1.GetNetworkStatsRequest{})
}

func (c *EnxameClient) ListNodes(filterType int, onlineOnly bool, limit, offset int) (*pbv1.ListNodesResponse, error) {
	return c.adminClient.ListNodes(c.withAuth(context.Background()), &pbv1.ListNodesRequest{
		FilterType: pbv1.NodeType(filterType),
		OnlineOnly: onlineOnly,
		Limit:      int32(limit),
		Offset:     int32(offset),
	})
}

func (c *EnxameClient) ListUsers(limit, offset int) (*pbv1.ListUsersResponse, error) {
	return c.adminClient.ListUsers(c.withAuth(context.Background()), &pbv1.ListUsersRequest{
		Limit:  int32(limit),
		Offset: int32(offset),
	})
}

func (c *EnxameClient) UpdateUserRole(userID, newRole string) (*pbv1.UpdateUserRoleResponse, error) {
	return c.adminClient.UpdateUserRole(c.withAuth(context.Background()), &pbv1.UpdateUserRoleRequest{
		UserId:  userID,
		NewRole: newRole,
	})
}

func (c *EnxameClient) BanNode(nodeID, reason string, durationSeconds int64) (*pbv1.BanNodeResponse, error) {
	return c.adminClient.BanNode(c.withAuth(context.Background()), &pbv1.BanNodeRequest{
		NodeId:          nodeID,
		Reason:          reason,
		DurationSeconds: durationSeconds,
	})
}

func (c *EnxameClient) UnbanNode(nodeID string) (*pbv1.UnbanNodeResponse, error) {
	return c.adminClient.UnbanNode(c.withAuth(context.Background()), &pbv1.UnbanNodeRequest{
		NodeId: nodeID,
	})
}

func (c *EnxameClient) SendGlobalAlert(title, message, severity, infoURL string) (*pbv1.SendGlobalAlertResponse, error) {
	return c.adminClient.SendGlobalAlert(c.withAuth(context.Background()), &pbv1.SendGlobalAlertRequest{
		Title:    title,
		Message:  message,
		Severity: severity,
		InfoUrl:  infoURL,
	})
}

func (c *EnxameClient) VerifyMasterKey(key string) (*pbv1.VerifyMasterKeyResponse, error) {
	resp, err := c.adminClient.VerifyMasterKey(c.withAuth(context.Background()), &pbv1.VerifyMasterKeyRequest{
		MasterKey: key,
	})
	if err == nil && resp.Success {
		c.vaultToken = resp.VaultToken

		// Autoclear after 30 mins
		go func() {
			time.Sleep(30 * time.Minute)
			c.vaultToken = ""
		}()
	}
	return resp, err
}

// Helper to get connection (internal use in SDK)
func (c *EnxameClient) getCoreConn() (*grpc.ClientConn, error) {
	if c.coreConn != nil {
		return c.coreConn, nil
	}
	// Fallback to connect if needed
	err := c.connectToCore()
	return c.coreConn, err
}

func (c *EnxameClient) CheckForUpdate() (*pbv1.CheckForUpdateResponse, error) {
	return c.updateClient.CheckForUpdate(c.withAuth(context.Background()), &pbv1.CheckForUpdateRequest{
		CurrentVersion: "1.0.0", // TODO: usar constante global
		Platform:       runtime.GOOS,
	})
}

func (c *EnxameClient) DoUpdate(url string) error {
	log.Printf("[Update] Initing update from %s", url)

	// 1. Download
	resp, err := http.Get(url)
	if err != nil {
		return fmt.Errorf("failed to download: %w", err)
	}
	defer resp.Body.Close()

	exePath, err := os.Executable()
	if err != nil {
		return err
	}
	dir := filepath.Dir(exePath)
	downloadPath := filepath.Join(dir, "enxame.download")

	out, err := os.Create(downloadPath)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, resp.Body)
	if err != nil {
		return err
	}
	out.Close()

	// 2. Swap (Atomic Rename)
	oldPath := exePath + ".old"
	_ = os.Remove(oldPath) // Limpa se já existir

	if err := os.Rename(exePath, oldPath); err != nil {
		return fmt.Errorf("failed to rename current exe: %w", err)
	}

	if err := os.Rename(downloadPath, exePath); err != nil {
		// Rollback se falhar
		os.Rename(oldPath, exePath)
		return fmt.Errorf("failed to replace exe: %w", err)
	}

	// 3. Restart
	log.Printf("[Update] Swap complete. Restarting...")
	cmd := exec.Command(exePath)

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to restart: %w", err)
	}

	os.Exit(0)
	return nil
}
