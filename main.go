package main

import (
	"opcuababy/internal/api"
	"opcuababy/internal/controller"
	"opcuababy/internal/ui"
)


func main() {
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