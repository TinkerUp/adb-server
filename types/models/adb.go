package models

import (
	"time"
)

type Device struct {
	Serial       string      `json:"device_id"`
	State        DeviceState `json:"status"`
	Model        string      `json:"model"`
	Manufacturer string      `json:"manufacturer"`
	IsAuthorized bool        `json:"authorized"`
}

type Package struct {
	Name     string `json:"pkg_name"`
	ApkPath  string `json:"pkg_path"`
	IsSystem bool   `json:"is_system"`
}

type ListPackageOptions struct {
	IncludeSystem      bool
	IncludeUninstalled bool
}

type DeviceState string

const (
	DeviceStateOffline      DeviceState = "offline"
	DeviceStateBooting      DeviceState = "booting"
	DeviceStateUnauthorized DeviceState = "unauthorized"
	DeviceStateOnline       DeviceState = "online"
	DeviceStateUnknown      DeviceState = "unknown"
)

type DeviceStateChange struct {
	Serial    string      `json:"serial"`
	OldState  DeviceState `json:"old_state"`
	NewState  DeviceState `json:"new_state"`
	Timestamp time.Time   `json:"timestamp"`
}
