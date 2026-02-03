# Enxame - Rede Social Descentralizada P2P 🐝

O **Enxame** é uma plataforma de comunicação descentralizada focada em privacidade, criptografia ponta-a-ponta (E2EE) e alta disponibilidade. O projeto utiliza uma arquitetura híbrida de Core Servers (orquestração), Relays (tráfego de mensagens) e Clientes Desktop (Wails/React).

## ✨ Principais Funcionalidades

- **Canais Criptografados**: Mensagens cifradas com AES-GCM e troca de chaves X25519.
- **Identidade de Canal**: Suporte a avatars e sistema de gerenciamento de canais (incluindo o canal oficial `#Inicio`).
- **Cluster Dinâmico (Alta Disponibilidade)**: Múltiplos Core Servers sincronizados com failover automático no cliente.
- **Comunicação P2P**: Mensageria direta e em grupo via rede de relays descentralizada.
- **Módulos Integrados**: Wiki colaborativa, Tópicos (Tags) e compartilhamento de arquivos chunked.
- **Grid Computing**: Sistema de processamento distribuído entre nós da rede.

## 🏗️ Arquitetura

- **gRPC**: Comunicação principal entre todos os componentes da malha.
- **PostgreSQL**: Persistência de metadados, governança e logs de auditoria.
- **Redis**: Estado em tempo real, presença de nós e barramento de eventos (Pub/Sub).
- **Wails & React**: Interface desktop moderna e de alto desempenho.
- **SQLite**: Persistência local no cliente para histórico e segredos.

## 🚀 Como Iniciar

### Pré-requisitos
- Go 1.22+
- Docker e Docker Compose
- Node.js & NPM (para o frontend)
- Ferramentas gRPC instaladas

### Executando a Hidra (Cluster de Cores)

1. **Inicie a infraestrutura base**:
   ```bash
   docker-compose up -d
   ```

2. **Inicie o Core Primário**:
   ```bash
   go run ./cmd/core-server
   ```

3. **Inicie o Cliente GUI**:
   ```bash
   cd cmd/gui
   wails dev
   ```

## 📂 Estrutura do Projeto

- `cmd/core-server/`: Orquestrador central da rede.
- `cmd/gui/`: Cliente desktop desenvolvido em Wails/React.
- `pkg/client_sdk/`: SDK em Go que abstrai toda a lógica de segurança, storage e rede para o cliente.
- `relay/`: Servidor de tráfego de mensagens pura (Stateless).
- `internal/server/`: Implementações manuais dos serviços gRPC (Cluster, Channel, Grid).
- `pkg/storage/`: Camada de persistência local (SQLite) do SDK.

## 🛠️ Configurações (Core Server)

| Variável | Descrição |
|----------|-----------|
| `GRPC_PORT` | Porta do servidor gRPC (Padrão: 50051) |
| `POSTGRES_HOST` | Host do banco de dados relacional |
| `REDIS_HOST` | Host do banco de dados em memória |
| `MASTER_PUBLIC_KEYS` | Chaves mestras de moderação (Admin Approval) |

## ⚖️ Licença

Este projeto está sob a licença MIT.
