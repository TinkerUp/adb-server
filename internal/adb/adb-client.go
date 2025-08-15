package adb

import (
	"context"
	"strings"
	"time"

	"github.com/TinkerUp/adb-server/types/models"
	adb "github.com/zach-klippenstein/goadb"
)

type ADBClient interface {
	Version(ctx context.Context) (int, error)

	Devices(ctx context.Context) ([]models.Device, error)
	TrackDeviceStates(ctx context.Context, deviceSerial string) (<-chan models.DeviceStateChange, error)

	Packages(ctx context.Context, deviceId string, opts models.ListPackageOptions) ([]models.Package, error)
	Install(ctx context.Context, deviceId string, pkgPath string) error
	Uninstall(ctx context.Context, deviceId string, pkgName string, keepData bool, user int) error

	Pull(ctx context.Context, serial, remotePath, localPath string) error
	Push(ctx context.Context, serial, localPath, remotePath string) error
}

type GoADBClient struct {
	adb *adb.Adb
}

func NewGoADBClient() (*GoADBClient, error) {
	client, err := adb.New()
	if err != nil {
		return nil, err
	}

	return &GoADBClient{
		adb: client,
	}, nil
}

func (client *GoADBClient) Version(ctx context.Context) (int, error) {
	return client.adb.ServerVersion()
}

func (client *GoADBClient) Devices(ctx context.Context) ([]models.Device, error) {
	devices, err := client.adb.ListDevices()
	if err != nil {
		return nil, err
	}

	devicesList := make([]models.Device, 0, len(devices))

	for _, deviceInfo := range devices {
		device := client.adb.Device(adb.DeviceWithSerial(deviceInfo.Serial))

		deviceState, stateErr := device.State()
		deviceManufacturer, manufacturerErr := GetDeviceManufacturer(device)

		if stateErr != nil {
			deviceState = adb.StateInvalid
		}

		if manufacturerErr != nil {
			deviceManufacturer = "Unknown"
		}

		standardizedState := ConvertState(deviceState)

		devicesList = append(devicesList, models.Device{
			Serial:       deviceInfo.Serial,
			State:        standardizedState,
			Model:        deviceInfo.Model,
			Manufacturer: deviceManufacturer,
			IsAuthorized: standardizedState == models.DeviceStateOnline,
		})
	}
	return devicesList, nil
}

func (client *GoADBClient) TrackDeviceStates(ctx context.Context, deviceSerial string) (<-chan models.DeviceStateChange, error) {
	goAdbChannel := client.adb.NewDeviceWatcher().C()

	stateChannel := make(chan models.DeviceStateChange)

	go func() {
		defer close(stateChannel)

		for watcher := range goAdbChannel {
			if watcher.Serial == deviceSerial {
				stateChange := models.DeviceStateChange{
					Serial:    deviceSerial,
					OldState:  ConvertState(watcher.OldState),
					NewState:  ConvertState(watcher.NewState),
					Timestamp: time.Now(),
				}
				stateChannel <- stateChange
			}
		}
	}()

	return stateChannel, nil
}

func GetDeviceManufacturer(device *adb.Device) (string, error) {
	manufacturer, err := device.RunCommand("getprop", "ro.product.manufacturer")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(manufacturer), nil
}

func ConvertState(state adb.DeviceState) models.DeviceState {
	switch state {
	case adb.StateOnline:
		return models.DeviceStateOnline
	case adb.StateOffline:
		return models.DeviceStateOffline
	case adb.StateUnauthorized:
		return models.DeviceStateUnauthorized
	default:
		return models.DeviceStateUnknown
	}
}
