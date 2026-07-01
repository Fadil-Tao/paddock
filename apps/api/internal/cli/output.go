package cli

import "time"

type responseWrapper[T any] struct {
	Message string `json:"message"`
	Data    T      `json:"data"`
}

type Port struct {
	Terminal string `json:"terminal"`
	VNC      string `json:"vnc"`
	CDP      string `json:"cdp"`
}

type Sandbox struct {
	ID          string    `json:"id"`
	ContainerId string    `json:"containerId,omitempty"`
	Name        string    `json:"name"`
	State       string    `json:"state"`
	Image       string    `json:"image"`
	Created     time.Time `json:"created"`
	LastExec    time.Time `json:"lastExec,omitempty"`
	Ports       Port      `json:"port,omitempty"`
	Engine      string    `json:"engine,omitempty"`
	NetworkId   string    `json:"networkId,omitempty"`
	VolumeName  string    `json:"volumeName,omitempty"`
}

type ExecResult struct {
	Stdout   string `json:"stdout"`
	Stderr   string `json:"stderr"`
	ExitCode int    `json:"exitCode"`
}

type LogsResult struct {
	Logs string `json:"logs"`
}
