package main

import (
	"encoding/json"
	"fmt"
	"opcuababy/internal/api"
	"opcuababy/internal/controller"
	"opcuababy/internal/ui"
	"os"
	"path/filepath"
)

func main() {
	// Attempt to set FYNE_SCALE from config before app initialization.
	// This fixes HiDPI issues on Linux by allowing user override in settings.
	trySetScaleEnv()

	c := controller.New()
	var apiStatus string

	// Inject the API server starter function into the controller
	// to break the import cycle.
	c.SetApiStarter(api.StartServer)

	ui := ui.NewUI(c, &apiStatus)

	// The controller is now responsible for starting the API server
	// based on the loaded configuration.
	c.SetApiStatus(&apiStatus)

	c.UpdateApiServerState(ui.GetConfig())

	ui.Run()
}

type scaleConfig struct {
	Scale float32 `json:"scale,omitempty"`
}

func trySetScaleEnv() {
	// If user explicitly set FYNE_SCALE or auto-scale behavior, respect it.
	if os.Getenv("FYNE_SCALE") != "" {
		return
	}

	exePath, err := os.Executable()
	if err != nil {
		return
	}
	configPath := filepath.Join(filepath.Dir(exePath), "opcuababy_config.json")
	data, err := os.ReadFile(configPath)
	if err != nil {
		return
	}
	var sc scaleConfig
	if err := json.Unmarshal(data, &sc); err == nil && sc.Scale > 0.1 {
		// Set the environment variable for Fyne
		os.Setenv("FYNE_SCALE", fmt.Sprintf("%g", sc.Scale))
	}
}
