package config

import (
	"github.com/kelseyhightower/envconfig"
)

// Config representa a configuração do Relay
type Config struct {
	// Endereço do Core Server (host:port)
	CoreAddress string `envconfig:"CORE_ADDRESS" default:"localhost:50051"`

	// IP público do relay (anunciado para outros nós)
	RelayIP string `envconfig:"RELAY_IP" required:"true"`

	// Porta gRPC do relay (para receber conexões de clientes)
	RelayPort int `envconfig:"RELAY_PORT" default:"50052"`

	// Caminho para o arquivo de chave privada
	PrivateKeyPath string `envconfig:"PRIVATE_KEY_PATH" default:"relay.key"`

	// Região do relay (para descoberta de serviço)
	Region string `envconfig:"RELAY_REGION" default:"default"`

	// Intervalo de heartbeat em segundos
	HeartbeatInterval int `envconfig:"HEARTBEAT_INTERVAL" default:"30"`

	// Capacidades do relay
	Capabilities []string `envconfig:"RELAY_CAPABILITIES" default:"relay,nat-traversal"`
}

// Load carrega a configuração das variáveis de ambiente
func Load() (*Config, error) {
	var cfg Config
	if err := envconfig.Process("", &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}
