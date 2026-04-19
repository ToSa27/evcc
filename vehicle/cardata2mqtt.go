package vehicle

import (
	"context"
	"errors"
	"strings"

	"github.com/evcc-io/evcc/api"
	"github.com/evcc-io/evcc/plugin/mqtt"
	"github.com/evcc-io/evcc/util"
	"github.com/evcc-io/evcc/vehicle/bmw/cardata"
)

// Cardata2Mqtt is an api.Vehicle implementation for BMW/Mini cars via custom MQTT broker
type Cardata2Mqtt struct {
	*embed
	*cardata.MqttProvider
}

func init() {
	registry.AddCtx("cardata2mqtt", NewCardata2MqttFromConfig)
}

// NewCardata2MqttFromConfig creates a new BMW/Mini CarData vehicle using a custom MQTT broker
func NewCardata2MqttFromConfig(ctx context.Context, other map[string]any) (api.Vehicle, error) {
	var cc struct {
		embed       `mapstructure:",squash"`
		mqtt.Config `mapstructure:",squash"`
		VIN         string
		Prefix      string
	}

	if err := util.DecodeOther(other, &cc); err != nil {
		return nil, err
	}

	if cc.VIN == "" {
		return nil, errors.New("missing vin")
	}

	// strip tcp:// scheme- evcc's MQTT client only handles tls:// and bare host:port
	cc.Config.Broker = strings.TrimPrefix(cc.Config.Broker, "tcp://")

	log := util.NewLogger("cardata2mqtt").Redact(cc.VIN, cc.Config.User, cc.Config.Password)

	client, err := mqtt.RegisteredClientOrDefault(log, cc.Config)
	if err != nil {
		return nil, err
	}

	provider, err := cardata.NewMqttProvider(log, client, cc.Prefix, cc.VIN)
	if err != nil {
		return nil, err
	}

	v := &Cardata2Mqtt{
		embed:        &cc.embed,
		MqttProvider: provider,
	}

	return v, nil
}
