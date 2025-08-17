package adb

import (
	"bufio"
	"context"
	"errors"
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
	type result struct {
		ver int
		err error
	}

	ch := make(chan result)

	go func() {
		version, err := client.adb.ServerVersion()
		ch <- result{
			ver: version,
			err: err,
		}
	}()

	select {
	case <-ctx.Done():
		return 0, ctx.Err()
	case result := <-ch:
		return result.ver, result.err
	}
}

func (client *GoADBClient) devices() ([]models.Device, error) {
	devices, err := client.adb.ListDevices()
	if err != nil {
		return nil, err
	}

	devicesList := make([]models.Device, 0, len(devices))

	for _, deviceInfo := range devices {
		device := client.getDevice(deviceInfo.Serial)

		deviceState, stateErr := device.State()
		var deviceManufacturer string

		if stateErr != nil {
			deviceState = adb.StateInvalid
		} else { // Why bother trying to get the manufacturer if the state is invalid
			deviceManufacturer, _ = client.getDeviceManufacturer(device)
		}

		if deviceManufacturer == "" {
			deviceManufacturer = "Unknown"
		}

		standardizedState := client.convertState(deviceState)

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

func (client *GoADBClient) Devices(ctx context.Context) ([]models.Device, error) {
	type result struct {
		devices []models.Device
		err     error
	}

	resultChannel := make(chan result)

	go func() {
		devices, err := client.devices()
		resultChannel <- result{
			devices: devices,
			err:     err,
		}
	}()

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case result := <-resultChannel:
		return result.devices, result.err
	}
}

func (client *GoADBClient) TrackDeviceStates(ctx context.Context, deviceSerial string) (<-chan models.DeviceStateChange, error) {
	goAdbChannel := client.adb.NewDeviceWatcher().C()

	stateChannel := make(chan models.DeviceStateChange)

	go func() {
		defer close(stateChannel)

		for {
			select {
			case <-ctx.Done():
				return
			case watcher, ok := <-goAdbChannel:
				if !ok {
					return
				}

				if watcher.Serial == deviceSerial {
					stateChange := models.DeviceStateChange{
						Serial:    deviceSerial,
						OldState:  client.convertState(watcher.OldState),
						NewState:  client.convertState(watcher.NewState),
						Timestamp: time.Now(),
					}

					select {
					case stateChannel <- stateChange:
					case <-ctx.Done():
						return
					}
				}
			}
		}
	}()

	return stateChannel, nil
}

func (client *GoADBClient) packages(deviceId string, opts models.ListPackageOptions) ([]models.Package, error) {
	device := client.getDevice(deviceId)

	if device == nil {
		return nil, errors.New("device not found")
	}

	args := []string{"-f"}

	if opts.IncludeUninstalled {
		args = append(args, "-u")
	}
	if opts.IncludeSystem {
		args = append(args, "-s")
	} else {
		args = append(args, "-3")
	}

	out, err := device.RunCommand("pm list packages", args...)

	if err != nil {
		return nil, err
	}

	var packages []models.Package

	scanner := bufio.NewScanner(strings.NewReader(out))

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		indexOfEqual := strings.LastIndex(line, "=")

		if line == "" || !strings.HasPrefix(line, "package:") || indexOfEqual == -1 {
			continue
		}

		pkgPath := strings.TrimPrefix(line[:indexOfEqual], "package:")
		pkgName := strings.TrimSpace(line[indexOfEqual+1:])

		packages = append(packages, models.Package{
			Name:     pkgName,
			ApkPath:  pkgPath,
			IsSystem: strings.Contains(pkgPath, "/system/"), // Sys apps generally are found in /system
		})
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	return packages, nil
}

func (client *GoADBClient) Packages(ctx context.Context, deviceId string, opts models.ListPackageOptions) ([]models.Package, error) {
	type result struct {
		pkgs []models.Package
		err  error
	}

	resultCh := make(chan result)

	go func() {
		packages, err := client.packages(deviceId, opts)
		resultCh <- result{
			pkgs: packages,
			err:  err,
		}
	}()

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case result := <-resultCh:
		return result.pkgs, result.err
	}
}

func (client *GoADBClient) getDevice(serial string) *adb.Device {
	return client.adb.Device(adb.DeviceWithSerial(serial))
}

func (client *GoADBClient) getDeviceManufacturer(device *adb.Device) (string, error) {
	manufacturer, err := device.RunCommand("getprop", "ro.product.manufacturer")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(manufacturer), nil
}

func (client *GoADBClient) convertState(state adb.DeviceState) models.DeviceState {
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
