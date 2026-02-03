# Build stage
FROM golang:1.24-alpine AS builder

RUN apk add --no-cache git ca-certificates tzdata

WORKDIR /app

# Copia go.mod e go.sum primeiro (cache de dependências)
COPY go.mod go.sum ./
RUN go mod download

# Copia o código fonte
COPY . .

# Build otimizado
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
    -ldflags="-w -s -extldflags '-static'" \
    -o /core-server \
    ./cmd/core-server

# Final stage - imagem mínima
FROM scratch

# Copia certificados CA e timezone
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=builder /usr/share/zoneinfo /usr/share/zoneinfo

# Copia o binário
COPY --from=builder /core-server /core-server

# Expõe a porta gRPC
EXPOSE 50051

# Executa como non-root (UID 1000)
USER 1000:1000

ENTRYPOINT ["/core-server"]
