package model

import "time"

type Port struct {
	Terminal string `json:"terminal"`
	VNC string `json:"vnc"`
	CDP string `json:"cdp"`
}

// type Resources struct {
// 	VCpu string `json:"vcpu"`
// 	Ram string `json:"ram"`
// 	Storage string `json:"storage"`
// }

type SandboxState string 

const (
    StateStopped SandboxState = "paused"
    StateRunning SandboxState = "running"
	StateRestarting SandboxState = "restarting"
	StateRemoving SandboxState = "removing"
	StateDead SandboxState = "dead"
)

type Sandbox struct {
	ID string `json:"id"`
	ContainerId string `json:"containerId,omitempty"`
	Name string `json:"name"`
	State SandboxState `json:"state"`
	Image string `json:"image"`
	Created time.Time `json:"created"`
	LastExec time.Time `json:"lastExec,omitempty"`
	Ports Port `json:"port,omitempty"`
	Engine string `json:"engine,omitempty"`
	NetworkId string `json:"networkId,omitempty"`
	VolumeName string `json:"volumeName,omitempty"`	
}

// id | name | state | image | created 