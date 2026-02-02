# Core Server - Rede Social Descentralizada P2P

Core Server em Go para orquestração de uma rede social descentralizada P2P.

## Arquitetura

- **gRPC**: Comunicação entre Core, Relays e Desktop Clients
- **Redis**: Estado em tempo real (heartbeats, presença, Pub/Sub)
- **PostgreSQL**: Persistência de metadados e histórico

## Primeiros Passos

### Pré-requisitos

- Go 1.22+
- Docker e Docker Compose
- protoc (Protocol Buffer Compiler)

### Instalando Ferramentas

```bash
make tools
```

### Gerando Código Protobuf

```bash
make proto
```

### Iniciando o Ambiente de Desenvolvimento

```bash
# Inicia toda a stack (Core Server + Postgres + Redis)
make docker-up

# Ou, para rodar localmente (apenas infraestrutura)
make docker-infra
make run
```

### Testando

```bash
# Lista serviços gRPC
make grpc-list

# Testa GetActiveRelays
make grpc-test
```

## Estrutura do Projeto

```
├── api/proto/v1/         # Definições Protobuf
├── cmd/core-server/      # Entrypoint
├── internal/
│   ├── config/           # Configurações
│   ├── crypto/           # Validação de assinaturas Ed25519
│   ├── domain/           # Entidades de domínio
│   ├── repository/       # Camada de dados (Redis/Postgres)
│   ├── server/           # Servidor gRPC
│   └── service/          # Lógica de negócio
├── pkg/pb/v1/            # Código Go gerado (protoc)
└── scripts/              # Scripts auxiliares
```

## Variáveis de Ambiente

| Variável | Padrão | Descrição |
|----------|--------|-----------|
| `GRPC_PORT` | 50051 | Porta do servidor gRPC |
| `POSTGRES_HOST` | localhost | Host do PostgreSQL |
| `REDIS_HOST` | localhost | Host do Redis |
| `HEARTBEAT_TTL` | 60s | TTL dos heartbeats |
| `MASTER_PUBLIC_KEYS` | - | Chaves públicas dos moderadores globais (hex) |

## Licença

MIT
