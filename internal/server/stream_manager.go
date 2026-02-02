package server

import (
	"hash/fnv"
	"sync"

	pbv1 "github.com/goautomatik/core-server/pkg/pb/v1"
)

const (
	// numShards define o número de shards para distribuir locks
	// 256 shards = cada shard gerencia ~40 streams em 10k conexões
	numShards = 256
)

// streamShard representa um shard de streams com seu próprio lock
type streamShard struct {
	mu      sync.RWMutex
	streams map[string]chan *pbv1.GlobalEvent
}

// StreamManager gerencia streams de eventos com sharding para alta concorrência
// Em vez de um único mutex para todas as streams (contenção máxima),
// usamos 256 shards independentes, permitindo paralelismo real.
//
// Benefícios:
//   - Reduz contenção em ~256x comparado a um mutex único
//   - Evita deadlock (locks independentes por shard)
//   - Escala linearmente com número de núcleos
type StreamManager struct {
	shards     [numShards]*streamShard
	bufferSize int
}

// NewStreamManager cria um novo gerenciador de streams
func NewStreamManager(bufferSize int) *StreamManager {
	sm := &StreamManager{
		bufferSize: bufferSize,
	}
	for i := 0; i < numShards; i++ {
		sm.shards[i] = &streamShard{
			streams: make(map[string]chan *pbv1.GlobalEvent),
		}
	}
	return sm
}

// getShard retorna o shard para um dado nodeID
func (sm *StreamManager) getShard(nodeID string) *streamShard {
	h := fnv.New32a()
	h.Write([]byte(nodeID))
	return sm.shards[h.Sum32()%numShards]
}

// Register registra uma nova stream para um nó
// Retorna o canal para receber eventos
func (sm *StreamManager) Register(nodeID string) chan *pbv1.GlobalEvent {
	eventChan := make(chan *pbv1.GlobalEvent, sm.bufferSize)

	shard := sm.getShard(nodeID)
	shard.mu.Lock()
	shard.streams[nodeID] = eventChan
	shard.mu.Unlock()

	return eventChan
}

// Unregister remove uma stream de um nó
func (sm *StreamManager) Unregister(nodeID string) {
	shard := sm.getShard(nodeID)
	shard.mu.Lock()
	if ch, exists := shard.streams[nodeID]; exists {
		close(ch)
		delete(shard.streams, nodeID)
	}
	shard.mu.Unlock()
}

// Broadcast envia um evento para todas as streams ativas
// Cada shard é processado de forma independente (não bloqueia outros shards)
func (sm *StreamManager) Broadcast(event *pbv1.GlobalEvent) {
	// Processa cada shard em paralelo
	var wg sync.WaitGroup
	wg.Add(numShards)

	for i := 0; i < numShards; i++ {
		go func(shard *streamShard) {
			defer wg.Done()

			shard.mu.RLock()
			for _, ch := range shard.streams {
				select {
				case ch <- event:
					// Enviado com sucesso
				default:
					// Canal cheio, evento descartado para evitar bloqueio
				}
			}
			shard.mu.RUnlock()
		}(sm.shards[i])
	}

	wg.Wait()
}

// BroadcastAsync envia um evento para todas as streams sem esperar
// Use quando latência é mais importante que garantia de entrega
func (sm *StreamManager) BroadcastAsync(event *pbv1.GlobalEvent) {
	for i := 0; i < numShards; i++ {
		go func(shard *streamShard) {
			shard.mu.RLock()
			for _, ch := range shard.streams {
				select {
				case ch <- event:
				default:
				}
			}
			shard.mu.RUnlock()
		}(sm.shards[i])
	}
}

// Count retorna o número total de streams ativas
func (sm *StreamManager) Count() int {
	total := 0
	for i := 0; i < numShards; i++ {
		sm.shards[i].mu.RLock()
		total += len(sm.shards[i].streams)
		sm.shards[i].mu.RUnlock()
	}
	return total
}

// Has verifica se um nó está registrado
func (sm *StreamManager) Has(nodeID string) bool {
	shard := sm.getShard(nodeID)
	shard.mu.RLock()
	_, exists := shard.streams[nodeID]
	shard.mu.RUnlock()
	return exists
}
