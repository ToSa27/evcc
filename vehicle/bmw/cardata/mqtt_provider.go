package cardata

import (
	"encoding/json"
	"maps"
	"strings"
	"time"

	"github.com/evcc-io/evcc/plugin/mqtt"
	"github.com/evcc-io/evcc/util"
)

// NewMqttProvider creates a Provider that subscribes to cardata topics
// on a custom MQTT broker instead of BMW's streaming endpoint.
// The broker is expected to publish the same StreamingMessage JSON payloads
// as BMW's CarData streaming, just at a configurable topic prefix.
// Topic pattern: {prefix}/{vin} (or {vin} if prefix is empty).
func NewMqttProvider(log *util.Logger, client *mqtt.Client, prefix, vin string) (*Provider, error) {
	v := &Provider{
		log:       log,
		streaming: make(map[string]StreamingData),
		rest:      make(map[string]TelematicData),
		updated:   time.Now(), // prevent REST container setup
	}

	topic := vin
	if prefix != "" {
		topic = strings.TrimRight(prefix, "/") + "/" + vin
	}

	log.DEBUG.Printf("subscribing to topic: %s", topic)

	if err := client.Listen(topic, func(payload string) {
		log.DEBUG.Printf("message received (%d bytes)", len(payload))

		var res StreamingMessage
		if err := json.Unmarshal([]byte(payload), &res); err != nil {
			log.ERROR.Printf("unmarshal failed: %v, payload: %s", err, payload)
			return
		}

		log.DEBUG.Printf("parsed message: vin=%s, keys=%d", res.Vin, len(res.Data))
		for key, val := range res.Data {
			log.TRACE.Printf("  data: %s = %v", key, val.Value)
		}

		v.mu.Lock()
		maps.Copy(v.streaming, res.Data)
		v.updated = time.Now()
		log.DEBUG.Printf("streaming data updated, total keys: %d", len(v.streaming))
		v.mu.Unlock()
	}); err != nil {
		log.ERROR.Printf("subscribe failed for topic %s: %v", topic, err)
		return nil, err
	}

	log.DEBUG.Printf("subscribed successfully to: %s", topic)

	return v, nil
}
