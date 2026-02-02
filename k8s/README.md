# Kubernetes Deployment

## Pré-requisitos
- Kubernetes 1.25+
- kubectl configurado
- kustomize (ou kubectl com suporte)

## Deploy Desenvolvimento

```bash
# Aplicar manifests
kubectl apply -k k8s/overlays/dev

# Verificar pods
kubectl get pods -n enxame

# Verificar logs do Core Server
kubectl logs -f deployment/dev-core-server -n enxame
```

## Deploy Produção

```bash
# Alterar secrets ANTES de aplicar!
# Edite k8s/base/core-server-secrets.yaml

kubectl apply -k k8s/overlays/prod
```

## Verificar Health

```bash
# Status dos pods
kubectl get pods -n enxame -w

# Health check do Core Server
kubectl exec -it deployment/dev-core-server -n enxame -- grpc_health_probe -addr=localhost:50051
```

## Arquitetura

```
┌─────────────────────────────────────────────────────────────┐
│                     Kubernetes Cluster                      │
│                                                             │
│  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐         │
│  │ Core Server │  │ Core Server │  │ Core Server │  HPA    │
│  │   (Pod 1)   │  │   (Pod 2)   │  │   (Pod 3)   │  3-10   │
│  └──────┬──────┘  └──────┬──────┘  └──────┬──────┘         │
│         └────────────────┼────────────────┘                 │
│                          │                                  │
│                    ┌─────┴─────┐                            │
│                    │  Service  │                            │
│                    │core-server│                            │
│                    └─────┬─────┘                            │
│         ┌────────────────┼────────────────┐                 │
│         │                │                │                 │
│  ┌──────┴──────┐  ┌──────┴──────┐  ┌──────┴──────┐         │
│  │   Relay 1   │  │   Relay 2   │  │   Relay N   │  HPA    │
│  └─────────────┘  └─────────────┘  └─────────────┘  5-50   │
│                                                             │
│  ┌─────────────┐  ┌─────────────┐                          │
│  │    Redis    │  │  PostgreSQL │  StatefulSets            │
│  │ (Persisted) │  │ (Persisted) │                          │
│  └─────────────┘  └─────────────┘                          │
└─────────────────────────────────────────────────────────────┘
```

## Variáveis de Ambiente

| Variável | Padrão | Descrição |
|----------|--------|-----------|
| GRPC_PORT | 50051 | Porta gRPC |
| ENVIRONMENT | production | Ambiente |
| POSTGRES_HOST | postgres | Host do PostgreSQL |
| REDIS_HOST | redis | Host do Redis |
