# Build stage
FROM golang:1.24-alpine AS builder

WORKDIR /app

# Copia go.mod e go.sum do core e relay
COPY go.mod go.sum ./
COPY relay/go.mod relay/go.mod

# Download das dependências
RUN go mod download

# Copia código fonte
COPY . .

# Build do relay
WORKDIR /app/relay
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /relay ./cmd/relay

# Final stage
FROM alpine:3.19

RUN apk --no-cache add ca-certificates

WORKDIR /app

COPY --from=builder /relay .

# Variáveis de ambiente padrão
ENV CORE_ADDRESS=core-server:50051
ENV RELAY_PORT=50052
ENV PRIVATE_KEY_PATH=/data/relay.key
ENV RELAY_REGION=default

# Volume para persistir chaves
VOLUME ["/data"]

EXPOSE 50052

CMD ["./relay"]
