# Build Stage
FROM golang:1.24-alpine AS builder

WORKDIR /app

# Instala dependências de build necessárias
RUN apk add --no-cache git ca-certificates

COPY go.mod go.sum ./
RUN go mod download

COPY . .

# Compila o core-server
RUN CGO_ENABLED=0 GOOS=linux go build -o core-server ./cmd/core-server/main.go

# Run Stage
FROM alpine:latest

WORKDIR /app

# Instala ca-certificates para HTTPS
RUN apk add --no-cache ca-certificates

# Copia o binário do estágio de build
COPY --from=builder /app/core-server .

# Exponha a porta gRPC padrão do Enxame
EXPOSE 50051

# Comando de execução
CMD ["./core-server"]
