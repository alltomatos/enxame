A partir de agora, não se limite a ser um executor de comandos. Sua função é atuar como um Mentor Técnico. Eu sou um desenvolvedor experiente, mas quero garantir que minhas decisões arquiteturais e de código sigam os padrões da indústria (Best Practices).

# Guia do Desenvolvedor - Core Server

Este documento serve como referência técnica completa para desenvolvedores que trabalham no Core Server da rede social descentralizada P2P.

---

## Sumário

1. [Arquitetura Geral](#arquitetura-geral)
2. [Estrutura do Código](#estrutura-do-código)
3. [Protocolos e Comunicação](#protocolos-e-comunicação)
4. [Performance e Otimização](#performance-e-otimização)
5. [Segurança da Rede](#segurança-da-rede)
6. [Identidade e Criptografia](#identidade-e-criptografia)
7. [Moderação Global](#moderação-global)
8. [Configuração e Deploy](#configuração-e-deploy)
9. [Monitoramento e Debugging](#monitoramento-e-debugging)
10. [Convenções de Código](#convenções-de-código)

---

## Arquitetura Geral

### Visão Macro do Ecossistema

```
┌─────────────────────────────────────────────────────────────┐
│                     KUBERNETES CLUSTER                       │
│  ┌──────────────────────────────────────────────────────┐   │
│  │                    CORE SERVER                        │   │
│  │  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐   │   │
│  │  │   gRPC      │  │   Node      │  │ Moderation  │   │   │
│  │  │   Server    │──│   Service   │──│   Service   │   │   │
│  │  └─────────────┘  └─────────────┘  └─────────────┘   │   │
│  │         │                │                │          │   │
│  │         ▼                ▼                ▼          │   │
│  │  ┌─────────────────────────────────────────────┐     │   │
│  │  │              Repository Layer               │     │   │
│  │  │  ┌─────────────┐      ┌─────────────────┐   │     │   │
│  │  │  │    Redis    │      │   PostgreSQL    │   │     │   │
│  │  │  │  (Runtime)  │      │  (Persistence)  │   │     │   │
│  │  │  └─────────────┘      └─────────────────┘   │     │   │
│  │  └─────────────────────────────────────────────┘     │   │
│  └──────────────────────────────────────────────────────┘   │
└─────────────────────────────────────────────────────────────┘
           │                    │                    │
           ▼                    ▼                    ▼
    ┌──────────┐         ┌──────────┐         ┌──────────┐
    │  Relay   │         │  Relay   │         │  Relay   │
    │ Server 1 │         │ Server 2 │         │ Server N │
    └──────────┘         └──────────┘         └──────────┘
           │                    │                    │
           ▼                    ▼                    ▼
    ┌──────────┐         ┌──────────┐         ┌──────────┐
    │ Desktop  │◄───────►│ Desktop  │◄───────►│ Desktop  │
    │ Client 1 │   P2P   │ Client 2 │   P2P   │ Client N │
    └──────────┘         └──────────┘         └──────────┘
```

### Responsabilidades dos Componentes

| Componente | Responsabilidade |
|------------|------------------|
| **Core Server** | Orquestração central, registro de nós, moderação global |
| **Redis** | Estado em tempo real, heartbeats (TTL), Pub/Sub para eventos |
| **PostgreSQL** | Persistência de metadados, histórico de moderação, auditoria |
| **Relay Servers** | NAT Traversal, cache de conteúdo, proxy P2P |
| **Desktop Clients** | Nós P2P reais, grid computing (1% CPU/RAM) |

---

## Estrutura do Código

### Arquitetura Limpa (Clean Architecture)

```
goautomatik/
├── api/                    # Contratos externos
│   └── proto/v1/           # Definições gRPC/Protobuf
│
├── cmd/                    # Entrypoints
│   └── core-server/        # main.go
│
├── internal/               # Código privado do módulo
│   ├── config/             # Configurações via env vars
│   ├── crypto/             # Criptografia Ed25519
│   ├── domain/             # Entidades de negócio (SEM dependências externas)
│   ├── repository/         # Acesso a dados (Redis, Postgres)
│   ├── server/             # Camada de transporte (gRPC)
│   └── service/            # Lógica de negócio (orquestração)
│
├── pkg/                    # Código público/compartilhável
│   └── pb/v1/              # Código Go gerado pelo protoc
│
└── docs/                   # Documentação técnica
```

### Fluxo de Dependências

```
    ┌─────────────────────────────────────────────┐
    │                   main.go                    │
    │              (Composition Root)              │
    └─────────────────────────────────────────────┘
                         │
                         ▼
    ┌─────────────────────────────────────────────┐
    │                   Server                     │
    │               (gRPC Handlers)                │
    └─────────────────────────────────────────────┘
                         │
                         ▼
    ┌─────────────────────────────────────────────┐
    │                  Service                     │
    │              (Business Logic)                │
    └─────────────────────────────────────────────┘
                         │
          ┌──────────────┴──────────────┐
          ▼                             ▼
    ┌───────────────┐           ┌───────────────┐
    │  Repository   │           │    Domain     │
    │ (Data Access) │           │  (Entities)   │
    └───────────────┘           └───────────────┘
```

> **Regra de Ouro**: Dependências sempre apontam para dentro (Domain não depende de nada).

---

## Protocolos e Comunicação

### gRPC - NetworkService

| RPC | Tipo | Casos de Uso |
|-----|------|--------------|
| `RegisterNode` | Unary | Cliente desktop/relay iniciando sessão |
| `Heartbeat` | Unary | Keep-alive a cada 30s (renovação TTL) |
| `GetActiveRelays` | Unary | Descoberta de relays para NAT Traversal |
| `SubscribeToGlobalEvents` | Server Stream | Moderação em tempo real |

### Fluxo de Registro de Nó

```
┌──────────┐                    ┌──────────────┐
│  Client  │                    │ Core Server  │
└────┬─────┘                    └──────┬───────┘
     │                                  │
     │  1. RegisterNode(identity, type) │
     │─────────────────────────────────►│
     │                                  │
     │      2. Verifica assinatura      │
     │      3. Verifica ban list        │
     │      4. Armazena no Redis (TTL)  │
     │      5. Persiste no Postgres     │
     │                                  │
     │  RegisterNodeResponse            │
     │  (heartbeat_interval, relays)    │
     │◄─────────────────────────────────│
     │                                  │
     │  6. Heartbeat (a cada 30s)       │
     │─────────────────────────────────►│
     │                                  │
     │  7. Renova TTL no Redis          │
     │◄─────────────────────────────────│
     │                                  │
```

### Formato das Mensagens

```protobuf
// Identidade baseada em chave pública
message NodeIdentity {
  string node_id = 1;      // SHA256(public_key)[:16] hex
  bytes public_key = 2;    // Ed25519 (32 bytes)
  bytes signature = 3;     // Assinatura do node_id
}
```

---

## Performance e Otimização

### Metas de Performance

| Métrica | Alvo | Atual |
|---------|------|-------|
| Conexões gRPC simultâneas | 10.000+ | - |
| Latência RegisterNode (p99) | < 50ms | - |
| Latência Heartbeat (p99) | < 10ms | - |
| Throughput GetActiveRelays | 5.000 req/s | - |

### Otimizações Implementadas

#### 1. Connection Pooling

```go
// Redis - Pool de 100 conexões
redis.NewClient(&redis.Options{
    PoolSize:     100,
    MinIdleConns: 10,
})

// PostgreSQL - Pool de 25 conexões
pgxpool.Config{
    MaxConns: 25,
    MinConns: 5,
}
```

#### 2. Configurações gRPC para Alta Concorrência

```go
grpc.NewServer(
    grpc.MaxConcurrentStreams(10000),    // 10k streams simultâneos
    grpc.MaxRecvMsgSize(4 * 1024 * 1024), // 4MB max message
    grpc.KeepaliveParams(keepalive.ServerParameters{
        MaxConnectionIdle:     5 * time.Minute,
        MaxConnectionAge:      30 * time.Minute,
        Time:                  1 * time.Minute,
        Timeout:               20 * time.Second,
    }),
)
```

#### 3. TTL Automático no Redis

```go
// Heartbeat com TTL - sem polling
client.Set(ctx, key, data, 60*time.Second)  // Auto-expire
```

#### 4. Pub/Sub para Eventos de Moderação

```go
// Publisher
client.Publish(ctx, "moderation_events", eventJSON)

// Subscriber (em cada stream gRPC)
pubsub := client.Subscribe(ctx, "moderation_events")
```

### Goroutines e Concorrência

- **Cada stream gRPC**: goroutine separada
- **Event Broadcaster**: goroutine dedicada para propagar eventos
- **Rate Limiter**: estrutura thread-safe com mutex

```go
// Padrão de broadcast para streams
for event := range eventChan {
    s.streamsMu.RLock()
    for _, ch := range s.activeStreams {
        select {
        case ch <- event:  // Non-blocking send
        default:           // Drop se buffer cheio
        }
    }
    s.streamsMu.RUnlock()
}
```

---

## Segurança da Rede

### Modelo de Ameaças

| Ameaça | Mitigação |
|--------|-----------|
| **Sybil Attack** | Identidade baseada em chave pública, rate limiting |
| **DDoS no Core** | Rate limiting, Kubernetes autoscaling |
| **Man-in-the-Middle** | TLS obrigatório (produção), assinaturas Ed25519 |
| **Spoofing de Nó** | Verificação de assinatura em cada registro |
| **Moderador Malicioso** | Múltiplas chaves mestres, auditoria imutável |

### Camadas de Segurança

```
┌────────────────────────────────────────────────┐
│              Camada de Transporte               │
│         TLS 1.3 (em produção)                   │
├────────────────────────────────────────────────┤
│              Camada de Identidade               │
│    Ed25519 - Assinatura de cada mensagem        │
├────────────────────────────────────────────────┤
│              Camada de Autorização              │
│    Verificação de ban list, rate limiting       │
├────────────────────────────────────────────────┤
│              Camada de Moderação                │
│    Chaves mestres, assinaturas verificadas      │
└────────────────────────────────────────────────┘
```

### Rate Limiting

```go
// Limite: 100 requests por minuto por NodeID
type RateLimiter struct {
    requests map[string][]time.Time
    limit    int           // 100
    window   time.Duration // 1 minute
}
```

---

## Identidade e Criptografia

### Identidade Soberana (Self-Sovereign Identity)

**Princípio**: Cada usuário controla sua própria identidade através de um par de chaves Ed25519.

```
┌─────────────────────────────────────────────────┐
│                GERAÇÃO DE IDENTIDADE             │
│                                                  │
│  1. Gera par Ed25519 (32 bytes priv, 32 pub)    │
│  2. node_id = hex(SHA256(public_key)[:16])      │
│  3. signature = Ed25519.Sign(private, node_id)  │
│                                                  │
│  Resultado: Identidade verificável sem servidor │
└─────────────────────────────────────────────────┘
```

### Verificação de Identidade

```go
// Fluxo de verificação
func VerifyNodeIdentity(nodeID string, publicKey, signature []byte) error {
    // 1. Verifica derivação do nodeID
    expectedID := DeriveNodeID(publicKey)
    if nodeID != expectedID {
        return errors.New("node_id does not match public key")
    }
    
    // 2. Verifica assinatura
    if !ed25519.Verify(publicKey, []byte(nodeID), signature) {
        return ErrSignatureInvalid
    }
    
    return nil
}
```

### Por que Ed25519?

| Característica | Valor |
|----------------|-------|
| Tamanho da chave | 32 bytes (compacto) |
| Tamanho da assinatura | 64 bytes |
| Velocidade de verificação | ~71.000/s (single core) |
| Segurança | 128 bits (equivalente a RSA 3072) |
| Resistência a ataques | Side-channel resistant |

---

## Moderação Global

### Arquitetura de Moderação

```
┌─────────────────────────────────────────────────┐
│              CHAVES MESTRES                      │
│  (Configuradas via MASTER_PUBLIC_KEYS)          │
│  Representam moderadores globais confiáveis      │
└─────────────────────────────────────────────────┘
                      │
                      ▼
┌─────────────────────────────────────────────────┐
│           AÇÃO DE MODERAÇÃO                      │
│  1. Moderador cria ação (ban, alert, etc.)      │
│  2. Assina com sua chave privada                │
│  3. Envia para Core Server                       │
└─────────────────────────────────────────────────┘
                      │
                      ▼
┌─────────────────────────────────────────────────┐
│           CORE SERVER                            │
│  1. Verifica se chave está em MASTER_KEYS       │
│  2. Verifica assinatura da ação                  │
│  3. Persiste no PostgreSQL (auditoria)          │
│  4. Propaga via Redis Pub/Sub                    │
└─────────────────────────────────────────────────┘
                      │
                      ▼
┌─────────────────────────────────────────────────┐
│           TODOS OS RELAYS                        │
│  (Recebem via SubscribeToGlobalEvents)          │
│  1. Aplicam ação localmente                      │
│  2. Propagam para clientes conectados           │
└─────────────────────────────────────────────────┘
```

### Tipos de Eventos

| Tipo | Descrição | Payload |
|------|-----------|---------|
| `NODE_BANNED` | Nó banido da rede | node_id, reason, duration |
| `CONTENT_BANNED` | Conteúdo banido | content_hash, category |
| `GLOBAL_ALERT` | Alerta global | title, message, severity |
| `RELAY_OFFLINE` | Relay caiu | relay_id, alternatives |
| `NETWORK_UPDATE` | Atualização de config | type, payload |

### Auditoria Imutável

Todas as ações de moderação são persistidas no PostgreSQL:

```sql
CREATE TABLE moderation_actions (
    action_id VARCHAR(64) PRIMARY KEY,
    event_type INTEGER NOT NULL,
    target_id VARCHAR(128) NOT NULL,
    reason TEXT,
    moderator_id VARCHAR(64) REFERENCES global_moderators,
    signature BYTEA NOT NULL,  -- Prova criptográfica
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);
```

---

## Configuração e Deploy

### Variáveis de Ambiente

| Variável | Padrão | Descrição |
|----------|--------|-----------|
| `GRPC_PORT` | 50051 | Porta do servidor gRPC |
| `ENVIRONMENT` | development | development / staging / production |
| `LOG_LEVEL` | info | debug / info / warn / error |
| `POSTGRES_HOST` | localhost | Host do PostgreSQL |
| `POSTGRES_POOL_SIZE` | 25 | Conexões no pool |
| `REDIS_HOST` | localhost | Host do Redis |
| `REDIS_POOL_SIZE` | 100 | Conexões no pool |
| `HEARTBEAT_TTL` | 60s | TTL de presença no Redis |
| `MASTER_PUBLIC_KEYS` | - | Chaves hex dos moderadores |

### Deploy com Docker Compose (Desenvolvimento)

```bash
# Subir todo o ambiente
make docker-up

# Apenas infraestrutura
make docker-infra

# Logs do Core Server
make logs
```

### Deploy com Kubernetes (Produção)

```yaml
# Exemplo de Deployment
apiVersion: apps/v1
kind: Deployment
metadata:
  name: core-server
spec:
  replicas: 3
  selector:
    matchLabels:
      app: core-server
  template:
    spec:
      containers:
      - name: core-server
        image: core-server:latest
        ports:
        - containerPort: 50051
        resources:
          requests:
            memory: "256Mi"
            cpu: "250m"
          limits:
            memory: "512Mi"
            cpu: "500m"
```

---

## Monitoramento e Debugging

### Logging Estruturado

```go
// Formato atual (básico)
log.Printf("[gRPC] %s | OK | %v", info.FullMethod, duration)

// TODO: Migrar para logging estruturado (zap/zerolog)
```

### Endpoints de Debug

```bash
# Listar serviços gRPC
grpcurl -plaintext localhost:50051 list

# Testar GetActiveRelays
grpcurl -plaintext -d '{}' localhost:50051 network.v1.NetworkService/GetActiveRelays

# Verificar Redis
docker-compose exec redis redis-cli KEYS "node:*"

# Verificar Postgres
docker-compose exec postgres psql -U core -d core_network -c "SELECT * FROM nodes;"
```

### Métricas Sugeridas (Prometheus)

```
# Conexões ativas
core_server_active_connections{type="desktop|relay"}

# Latência de RPCs
core_server_rpc_duration_seconds{method="RegisterNode|Heartbeat"}

# Taxa de erros
core_server_rpc_errors_total{method="...", code="..."}

# Eventos de moderação
core_server_moderation_events_total{type="NODE_BANNED|..."}
```

---

## Convenções de Código

### Nomenclatura

| Tipo | Convenção | Exemplo |
|------|-----------|---------|
| Packages | lowercase, singular | `domain`, `service` |
| Interfaces | sufixo -er ou Cap | `Repository`, `Verifier` |
| Structs | PascalCase | `NodeService`, `GRPCServer` |
| Métodos | PascalCase (exported) | `RegisterNode`, `Heartbeat` |
| Constantes | PascalCase ou UPPER_SNAKE | `NodeTypeDesktop`, `MAX_RETRIES` |

### Tratamento de Erros

```go
// Erros de domínio definidos como variáveis
var (
    ErrNodeNotFound     = errors.New("node not found")
    ErrInvalidSignature = errors.New("invalid signature")
    ErrNodeBanned       = errors.New("node is banned")
)

// Wrapping com contexto
if err := doSomething(); err != nil {
    return fmt.Errorf("failed to do something: %w", err)
}
```

### Testes

```bash
# Rodar todos os testes
make test

# Com coverage
go test -v -race -coverprofile=coverage.out ./...
go tool cover -html=coverage.out
```

### Commits

```
feat: add new RPC for node statistics
fix: correct TTL calculation in heartbeat
docs: update developer guide
refactor: extract signature verification
test: add unit tests for NodeService
```

---

## Roadmap Técnico

### Fase 1 - MVP ✅
- [x] Servidor gRPC básico
- [x] Registro de nós
- [x] Heartbeat com TTL
- [x] Moderação global

### Fase 2 - Robustez
- [ ] Métricas Prometheus
- [ ] Tracing distribuído (OpenTelemetry)
- [ ] TLS em produção
- [ ] Rate limiting por IP

### Fase 3 - Escala
- [ ] Sharding de Redis
- [ ] Read replicas PostgreSQL
- [ ] Horizontal Pod Autoscaler
- [ ] gRPC Load Balancing

---

> **Dúvidas?** Abra uma issue ou consulte o código fonte em `/internal`.
