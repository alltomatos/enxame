# Enxame Relay Server

Servidor Relay para a rede Enxame. Conecta-se ao Core Server, registra-se na rede e escuta eventos de moderação global.

## Configuração

| Variável | Descrição | Padrão |
|----------|-----------|--------|
| `CORE_ADDRESS` | Endereço do Core Server | `localhost:50051` |
| `RELAY_IP` | IP público do relay (obrigatório) | - |
| `RELAY_PORT` | Porta gRPC do relay | `50052` |
| `PRIVATE_KEY_PATH` | Caminho da chave privada | `relay.key` |
| `RELAY_REGION` | Região do relay | `default` |
| `HEARTBEAT_INTERVAL` | Intervalo de heartbeat (s) | `30` |

## Executar Local

```bash
# Com Core Server rodando localmente
RELAY_IP=127.0.0.1 go run ./cmd/relay
```

## Docker

```bash
docker build -t enxame-relay -f relay/Dockerfile .
docker run -e RELAY_IP=192.168.1.100 -e CORE_ADDRESS=core:50051 enxame-relay
```

## Ciclo de Vida

1. **Inicialização**: Carrega/gera par de chaves Ed25519
2. **Registro**: Chama `RegisterNode` no Core Server
3. **Heartbeat**: Loop a cada 30s para manter registro ativo
4. **Eventos**: Escuta `SubscribeToGlobalEvents` para moderação

## Arquitetura

```
┌─────────────┐         ┌─────────────┐
│   RELAY     │──gRPC──▶│  CORE       │
│  (Client)   │         │  (Server)   │
│             │◀─stream─│             │
│  Ed25519    │         │  Redis/PG   │
└─────────────┘         └─────────────┘
```
