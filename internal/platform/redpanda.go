package platform

import (
	serviceredpanda "github.com/endge-lab/service-kit-go/pkg/redpanda"
	"github.com/endge-lab/service-template-go/internal/config"

	"github.com/segmentio/kafka-go"
	"go.opentelemetry.io/otel"
	"go.uber.org/zap"
)

var ErrRedpandaDisabled = serviceredpanda.ErrDisabled

type RedpandaClient struct {
	client *serviceredpanda.Client
}

func NewRedpandaClient(cfg *config.Config, logger *zap.Logger) *RedpandaClient {
	return &RedpandaClient{
		client: serviceredpanda.NewClient(serviceredpanda.Config{
			Enabled:          cfg.Redpanda.Enabled,
			Brokers:          cfg.Redpanda.BrokerList(),
			ClientID:         cfg.Redpanda.ClientID,
			DialTimeout:      cfg.Redpanda.DialTimeout,
			ReadBatchTimeout: cfg.Redpanda.ReadBatchTimeout,
			WriteTimeout:     cfg.Redpanda.WriteTimeout,
		}, logger, otel.GetTextMapPropagator()),
	}
}

func (c *RedpandaClient) Enabled() bool {
	return c != nil && c.client != nil && c.client.Enabled()
}

func (c *RedpandaClient) NewReader(topic string, groupID string) (*kafka.Reader, error) {
	return c.client.NewReader(topic, groupID)
}

func (c *RedpandaClient) NewWriter(topic string) (*kafka.Writer, error) {
	return c.client.NewWriter(topic)
}
