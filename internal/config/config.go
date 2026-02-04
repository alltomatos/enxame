package config

import (
	"fmt"
	"net/url"
	"strconv"
	"time"

	"github.com/kelseyhightower/envconfig"
)

// Config contém todas as configurações do Core Server
type Config struct {
	// Server
	GRPCPort    int    `envconfig:"GRPC_PORT" default:"50051"`
	Environment string `envconfig:"ENVIRONMENT" default:"development"`
	LogLevel    string `envconfig:"LOG_LEVEL" default:"info"`

	// PostgreSQL
	PostgresHost     string `envconfig:"POSTGRES_HOST" default:"localhost"`
	PostgresPort     int    `envconfig:"POSTGRES_PORT" default:"5432"`
	PostgresUser     string `envconfig:"POSTGRES_USER" default:"core"`
	PostgresPassword string `envconfig:"POSTGRES_PASSWORD" default:"core_secret"`
	PostgresDB       string `envconfig:"POSTGRES_DB" default:"core_network"`
	PostgresSSL      string `envconfig:"POSTGRES_SSL" default:"disable"`
	PostgresPoolSize int    `envconfig:"POSTGRES_POOL_SIZE" default:"25"`

	// Redis
	RedisHost     string `envconfig:"REDIS_HOST" default:"localhost"`
	RedisPort     int    `envconfig:"REDIS_PORT" default:"6379"`
	RedisPassword string `envconfig:"REDIS_PASSWORD" default:""`
	RedisDB       int    `envconfig:"REDIS_DB" default:"0"`
	RedisPoolSize int    `envconfig:"REDIS_POOL_SIZE" default:"100"`

	// Node Management
	HeartbeatInterval    time.Duration `envconfig:"HEARTBEAT_INTERVAL" default:"30s"`
	HeartbeatTTL         time.Duration `envconfig:"HEARTBEAT_TTL" default:"60s"`
	MaxRelaysPerResponse int           `envconfig:"MAX_RELAYS_PER_RESPONSE" default:"10"`

	// Moderation
	MasterPublicKeys []string `envconfig:"MASTER_PUBLIC_KEYS" default:""`
}

// Load carrega as configurações a partir de variáveis de ambiente
func Load() (*Config, error) {
	cfg := &Config{}
	if err := envconfig.Process("", cfg); err != nil {
		return nil, err
	}
	return cfg, nil
}

// PostgresDSN retorna a string de conexão do PostgreSQL de forma segura
func (c *Config) PostgresDSN() string {
	u := url.URL{
		Scheme: "postgres",
		User:   url.UserPassword(c.PostgresUser, c.PostgresPassword),
		Host:   fmt.Sprintf("%s:%d", c.PostgresHost, c.PostgresPort),
		Path:   "/" + c.PostgresDB,
	}
	q := u.Query()
	q.Set("sslmode", c.PostgresSSL)
	u.RawQuery = q.Encode()

	return u.String()
}

// RedisAddr retorna o endereço do Redis
func (c *Config) RedisAddr() string {
	return c.RedisHost + ":" + strconv.Itoa(c.RedisPort)
}
