package cardata

import (
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/evcc-io/evcc/api"
	"github.com/evcc-io/evcc/plugin/mqtt"
	"github.com/evcc-io/evcc/util"
	"github.com/spf13/cast"
)

// MqttProvider implements the vehicle api by subscribing to cardata
// topics on a custom MQTT broker instead of BMW's streaming endpoint.
type MqttProvider struct {
	mu   sync.Mutex
	log  *util.Logger
	data map[string]string
}

// NewMqttProvider creates a vehicle api provider that subscribes to cardata
// topics on a custom MQTT broker. Topics are structured as {prefix}/{vin}/{key}.
func NewMqttProvider(log *util.Logger, client *mqtt.Client, prefix, vin string) (*MqttProvider, error) {
	v := &MqttProvider{
		log:  log,
		data: make(map[string]string),
	}

	base := vin
	if prefix != "" {
		base = strings.TrimRight(prefix, "/") + "/" + vin
	}

	for _, key := range requiredKeys {
		topic := base + "/" + key
		if err := client.Listen(topic, v.handler(key)); err != nil {
			return nil, err
		}
	}

	return v, nil
}

func (v *MqttProvider) handler(key string) func(string) {
	return func(payload string) {
		v.mu.Lock()
		v.data[key] = payload
		v.mu.Unlock()
	}
}

func (v *MqttProvider) any(key string) (any, error) {
	v.mu.Lock()
	defer v.mu.Unlock()

	if val, ok := v.data[key]; ok {
		return val, nil
	}

	return nil, api.ErrNotAvailable
}

func (v *MqttProvider) stringVal(key string) (string, error) {
	res, err := v.any(key)
	if err != nil {
		return "", err
	}
	return cast.ToStringE(res)
}

func (v *MqttProvider) intVal(key string) (int64, error) {
	res, err := v.any(key)
	if err != nil {
		return 0, err
	}
	return cast.ToInt64E(res)
}

func (v *MqttProvider) floatVal(key string) (float64, error) {
	res, err := v.any(key)
	if err != nil {
		return 0, err
	}
	return cast.ToFloat64E(res)
}

var _ api.Battery = (*MqttProvider)(nil)

// Soc implements the api.Battery interface
func (v *MqttProvider) Soc() (float64, error) {
	return v.floatVal("vehicle.drivetrain.batteryManagement.header")
}

var _ api.ChargeState = (*MqttProvider)(nil)

// Status implements the api.ChargeState interface
func (v *MqttProvider) Status() (api.ChargeStatus, error) {
	port, err := v.stringVal("vehicle.body.chargingPort.status")
	if err != nil {
		return api.StatusNone, err
	}

	status := api.StatusA // disconnected
	if port == "CONNECTED" {
		status = api.StatusB
	}

	cs, err := v.stringVal("vehicle.drivetrain.electricEngine.charging.status")
	if err != nil || cs == "" {
		cs, err = v.stringVal("vehicle.drivetrain.electricEngine.charging.hvStatus")
	}

	if slices.Contains([]string{
		"CHARGINGACTIVE", // vehicle.drivetrain.electricEngine.charging.status
		"CHARGING",       // vehicle.drivetrain.electricEngine.charging.hvStatus
	}, cs) {
		return api.StatusC, nil
	}

	return status, err
}

var _ api.VehicleFinishTimer = (*MqttProvider)(nil)

// FinishTime implements the api.VehicleFinishTimer interface
func (v *MqttProvider) FinishTime() (time.Time, error) {
	res, err := v.intVal("vehicle.drivetrain.electricEngine.charging.timeRemaining")
	return time.Now().Add(time.Duration(res) * time.Minute), err
}

var _ api.VehicleRange = (*MqttProvider)(nil)

// Range implements the api.VehicleRange interface
func (v *MqttProvider) Range() (int64, error) {
	return v.intVal("vehicle.drivetrain.electricEngine.kombiRemainingElectricRange")
}

var _ api.VehicleOdometer = (*MqttProvider)(nil)

// Odometer implements the api.VehicleOdometer interface
func (v *MqttProvider) Odometer() (float64, error) {
	return v.floatVal("vehicle.vehicle.travelledDistance")
}

var _ api.SocLimiter = (*MqttProvider)(nil)

// GetLimitSoc implements the api.SocLimiter interface
func (v *MqttProvider) GetLimitSoc() (int64, error) {
	return v.intVal("vehicle.powertrain.electric.battery.stateOfCharge.target")
}

var _ api.VehicleClimater = (*MqttProvider)(nil)

// Climater implements the api.VehicleClimater interface
func (v *MqttProvider) Climater() (bool, error) {
	activeStates := []string{"HEATING", "COOLING", "VENTILATION", "DEFROST"}

	res, err := v.stringVal("vehicle.cabin.hvac.preconditioning.status.comfortState")
	if err == nil && res != "" {
		return slices.Contains(activeStates, strings.TrimPrefix(strings.ToUpper(res), "COMFORT_")), nil
	}

	if res, err = v.stringVal("vehicle.vehicle.preConditioning.activity"); err == nil {
		return slices.Contains(activeStates, strings.ToUpper(res)), nil
	}

	return false, err
}

var _ api.VehiclePosition = (*MqttProvider)(nil)

// Position implements the api.VehiclePosition interface
func (v *MqttProvider) Position() (float64, float64, error) {
	lat, err := v.floatVal("vehicle.cabin.infotainment.navigation.currentLocation.latitude")
	if err != nil {
		return 0, 0, err
	}

	lon, err := v.floatVal("vehicle.cabin.infotainment.navigation.currentLocation.longitude")
	if err != nil {
		return 0, 0, err
	}

	return lat, lon, nil
}
