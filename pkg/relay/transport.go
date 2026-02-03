package relay

import (
	"encoding/json"

	pbv1 "github.com/goautomatik/core-server/pkg/pb/v1"
)

// Transport types
const (
	CommandTypeEnvelope = "envelope"
)

// EnvelopeToCommand converte Envelope para NodeCommand (transporte)
func EnvelopeToCommand(env *pbv1.Envelope) (*pbv1.NodeCommand, error) {
	data, err := json.Marshal(env)
	if err != nil {
		return nil, err
	}
	return &pbv1.NodeCommand{
		CommandId:   env.MessageId,
		CommandType: CommandTypeEnvelope,
		Payload:     data,
	}, nil
}

// CommandToEnvelope converte NodeCommand para Envelope
func CommandToEnvelope(cmd *pbv1.NodeCommand) (*pbv1.Envelope, error) {
	if cmd.CommandType != CommandTypeEnvelope {
		return nil, nil // Ignora outros comandos por enquanto
	}
	env := &pbv1.Envelope{}
	if err := json.Unmarshal(cmd.Payload, env); err != nil {
		return nil, err
	}
	return env, nil
}
