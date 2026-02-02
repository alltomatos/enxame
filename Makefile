.PHONY: proto build run test clean docker-build docker-up docker-down deps

# Variáveis
BINARY_NAME=core-server
PROTO_DIR=api/proto/v1
PB_OUT_DIR=pkg/pb/v1

# Gera código Go a partir dos arquivos .proto
proto:
	@echo "Generating protobuf code..."
	@mkdir -p $(PB_OUT_DIR)
	protoc \
		--proto_path=$(PROTO_DIR) \
		--go_out=$(PB_OUT_DIR) --go_opt=paths=source_relative \
		--go-grpc_out=$(PB_OUT_DIR) --go-grpc_opt=paths=source_relative \
		$(PROTO_DIR)/*.proto
	@echo "Protobuf code generated in $(PB_OUT_DIR)"

# Instala dependências
deps:
	@echo "Downloading dependencies..."
	go mod download
	go mod tidy

# Build do binário
build: deps
	@echo "Building $(BINARY_NAME)..."
	CGO_ENABLED=0 go build -ldflags="-w -s" -o bin/$(BINARY_NAME) ./cmd/core-server
	@echo "Build complete: bin/$(BINARY_NAME)"

# Executa o servidor localmente
run: deps
	@echo "Running $(BINARY_NAME)..."
	go run ./cmd/core-server

# Executa testes
test:
	@echo "Running tests..."
	go test -v -race -cover ./...

# Limpa artefatos de build
clean:
	@echo "Cleaning..."
	rm -rf bin/
	rm -rf $(PB_OUT_DIR)/*.go
	go clean

# Build da imagem Docker
docker-build:
	@echo "Building Docker image..."
	docker build -t core-server:latest .

# Sobe o ambiente de desenvolvimento
docker-up:
	@echo "Starting development environment..."
	docker-compose up -d --build
	@echo "Environment started. Core Server running on localhost:50051"

# Para o ambiente de desenvolvimento
docker-down:
	@echo "Stopping development environment..."
	docker-compose down

# Sobe apenas infraestrutura (postgres + redis)
docker-infra:
	@echo "Starting infrastructure..."
	docker-compose up -d postgres redis
	@echo "Postgres: localhost:5432, Redis: localhost:6379"

# Logs do Core Server
logs:
	docker-compose logs -f core-server

# Verifica conexão com Redis
redis-cli:
	docker-compose exec redis redis-cli

# Verifica conexão com Postgres
psql:
	docker-compose exec postgres psql -U core -d core_network

# Instala ferramentas de desenvolvimento
tools:
	@echo "Installing development tools..."
	go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
	go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest
	go install github.com/fullstorydev/grpcurl/cmd/grpcurl@latest
	@echo "Tools installed. Make sure protoc is in your PATH."

# Verifica endpoints gRPC
grpc-list:
	grpcurl -plaintext localhost:50051 list

# Teste rápido de GetActiveRelays
grpc-test:
	grpcurl -plaintext -d '{}' localhost:50051 network.v1.NetworkService/GetActiveRelays

# Help
help:
	@echo "Available targets:"
	@echo "  proto       - Generate protobuf code"
	@echo "  deps        - Download dependencies"
	@echo "  build       - Build the binary"
	@echo "  run         - Run locally"
	@echo "  test        - Run tests"
	@echo "  clean       - Clean build artifacts"
	@echo "  docker-build - Build Docker image"
	@echo "  docker-up   - Start full environment"
	@echo "  docker-down - Stop environment"
	@echo "  docker-infra - Start only Postgres and Redis"
	@echo "  logs        - View Core Server logs"
	@echo "  tools       - Install development tools"
	@echo "  grpc-list   - List gRPC services"
	@echo "  grpc-test   - Test GetActiveRelays RPC"
