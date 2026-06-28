package runner

import (
	"bytes"
	"context"
	"fmt"
	"net/netip"
	"strconv"
	"time"

	"github.com/Fadil-Tao/paddock/internal/model"
	"github.com/matoous/go-nanoid/v2"
	"github.com/moby/moby/api/pkg/stdcopy"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/mount"
	"github.com/moby/moby/api/types/network"
	"github.com/moby/moby/client"
	"github.com/rs/zerolog/log"
)

// TODO: use sqlite later, optimize operation that using id

const image = "paddock/sandbox:dev"

type DockerRunner struct { 
	client *client.Client
}

func NewDockerRunner(c *client.Client) *DockerRunner { 
	return &DockerRunner{client:  c}
}


func (r *DockerRunner) Create(ctx context.Context ) (*model.Sandbox, error) {
	id, err := gonanoid.New(6)
	if err != nil {
		log.Err(err)
		return nil, err
	}

	networkName := "paddock-net-" + id
	volumeName := "paddock-vol-" + id

	netResult, err := r.client.NetworkCreate(ctx, networkName, client.NetworkCreateOptions{})
	if err != nil {
		log.Err(err).Msg("failed to create sandbox network")
		return nil, err
	}

	_, err = r.client.VolumeCreate(ctx, client.VolumeCreateOptions{Name: volumeName })
	if err != nil {
		r.client.NetworkRemove(ctx, netResult.ID , client.NetworkRemoveOptions{})
		log.Err(err).Msg("failed to creaete sandbox volume")
		return nil, err
	}

	portVNC, _ := network.ParsePort("6080/tcp")
	portTerm, _ := network.ParsePort("7681/tcp")
	portCDP , _ := network.ParsePort("9222/tcp")
	

	result, err := r.client.ContainerCreate(ctx, client.ContainerCreateOptions{
		Name:  "paddock" + id,
		Image: image,
		Config: &container.Config{
			Labels:  map[string]string{"paddock.id" : id},
			ExposedPorts: network.PortSet{portVNC : {}, portTerm: {}, portCDP: {}},
		},
		HostConfig: &container.HostConfig{
			PortBindings: network.PortMap{
				portVNC: []network.PortBinding{{HostIP: netip.Addr{}, HostPort: "0"}},
				portTerm: []network.PortBinding{{HostIP: netip.Addr{}, HostPort: "0"}},
				portCDP: []network.PortBinding{{HostIP: netip.Addr{}, HostPort: "0"}},		
			},
			Mounts:  []mount.Mount{{
				Type:  mount.TypeVolume,
				Source: volumeName,
				Target: "/workspace",
			}},
		},
	})

	if err != nil {
		r.client.NetworkRemove(ctx, netResult.ID, client.NetworkRemoveOptions{})
		r.client.VolumeRemove(ctx, volumeName, client.VolumeRemoveOptions{Force: true})
		log.Err(err).Msg("failed to create container")
		return nil, err
	}

	if _, err = r.client.ContainerStart(ctx, result.ID, client.ContainerStartOptions{}); err != nil {
		r.client.ContainerRemove(ctx, result.ID, client.ContainerRemoveOptions{})
		r.client.VolumeRemove(ctx, volumeName, client.VolumeRemoveOptions{})
		r.client.NetworkRemove(ctx, netResult.ID, client.NetworkRemoveOptions{})
		log.Err(err).Msg("failed to start container")
		return nil, err
	}

	info, err := r.client.ContainerInspect(ctx, result.ID, client.ContainerInspectOptions{})
	if err != nil{
		log.Err(err).Msg("failed to get container info")
		return nil, err
	}

	return &model.Sandbox{
		ID:  id,
		ContainerId: result.ID,
		Name: info.Container.Name,
		State: model.SandboxState("running"),
		Image: image,
		Ports:  model.Port{
			Terminal: hostPort(info.Container.NetworkSettings.Ports, portTerm),
			VNC:  hostPort(info.Container.NetworkSettings.Ports, portVNC),
			CDP: hostPort(info.Container.NetworkSettings.Ports, portCDP),
		},	
		Engine: "docker",
		NetworkId: netResult.ID,
		VolumeName: volumeName,
		Created: time.Now(),
	}, err
}


func (r *DockerRunner) List(ctx context.Context) (*[]model.Sandbox, error) {
	result, err := r.client.ContainerList(ctx, client.ContainerListOptions{
		All: true,
		Filters: make(client.Filters).Add("label", "paddock.id"),
	})
	if err != nil{
		log.Err(err).Msg("failed to list container")
		return nil, err
	}

	items := result.Items

	var sandboxList []model.Sandbox
	for _, item := range items{
		sandbox := model.Sandbox{
			ID:  item.Labels["paddock.id"],
			Name: item.Names[0],
			State: model.SandboxState(item.State),
			Image: item.Image,
			Created: time.Unix(item.Created, 0),
		}
		sandboxList = append(sandboxList, sandbox)
	}
	
	return &sandboxList, nil
}

func (r *DockerRunner) Get(ctx context.Context, id string) ( *model.Sandbox,error){
	result, err := r.client.ContainerList(ctx, client.ContainerListOptions{
		Filters: make(client.Filters).Add("label", "paddock.id=" + id),
		All: true,
	})

	if err != nil {
		log.Err(err).Msg("failed to get sandbox")
		return nil , err
	}

	if len(result.Items) <= 0 {
		return nil, model.ErrSandboxNotFound
	}

	item := result.Items[0]

	info, err := r.client.ContainerInspect(ctx, item.ID, client.ContainerInspectOptions{})
	
	if err != nil {
		log.Err(err).Msg("failed to get container info")
		return nil , err
	}

	portVNC, _ := network.ParsePort("6080/tcp")
	portTerm, _ := network.ParsePort("7681/tcp")
	portCDP , _ := network.ParsePort("9222/tcp")

	networkID := ""
    if net, ok := info.Container.NetworkSettings.Networks["paddock-net-"+id]; ok {
        networkID = net.NetworkID
    }

	volumeName := ""
	for _, m := range info.Container.HostConfig.Mounts {
		if m.Target == "/workspace" {
				volumeName = m.Source
				break
		}
	}

	return &model.Sandbox{
		ID:  id,
		ContainerId: info.Container.ID,
		Name: info.Container.Name,
		State: model.SandboxState(info.Container.State.Status),
		Image: info.Container.Config.Image,
		Created:  time.Unix(item.Created, 0) ,
		Ports:  model.Port{
			Terminal: hostPort(info.Container.NetworkSettings.Ports, portTerm),
			VNC:  hostPort(info.Container.NetworkSettings.Ports, portVNC),
			CDP: hostPort(info.Container.NetworkSettings.Ports, portCDP),
		},	
		Engine: "docker",
		NetworkId: networkID,
		VolumeName: volumeName,
	} , nil
} 

func (r  *DockerRunner) Remove(ctx context.Context, id string) error {
	result, err := r.client.ContainerList(ctx, client.ContainerListOptions{
		Filters: make(client.Filters).Add("label", "paddock.id=" + id),
		All: true,
	})

	if err != nil {
		log.Err(err).Msg("failed to get sandbox")
		return err
	}

	if len(result.Items) <= 0 {
		return model.ErrSandboxNotFound
	}

	item := result.Items[0]
	
	info, err := r.client.ContainerInspect(ctx, item.ID, client.ContainerInspectOptions{})
	
	if err != nil {
		log.Err(err).Msg("failed to get sandbox info")
		return err
	}

	networkID := ""
    if net, ok := info.Container.NetworkSettings.Networks["paddock-net-"+id]; ok {
        networkID = net.NetworkID
    }

	volumeName := ""
	for _, m := range info.Container.HostConfig.Mounts {
		if m.Target == "/workspace" {
				volumeName = m.Source
				break
		}
	}

	r.client.ContainerRemove(ctx, item.ID, client.ContainerRemoveOptions{
		Force:  true,
	})
	r.client.NetworkRemove(ctx, networkID, client.NetworkRemoveOptions{})
	r.client.VolumeRemove(ctx, volumeName, client.VolumeRemoveOptions{})
	
	return nil
} 

func (r *DockerRunner) Stop(ctx context.Context, id string) error {
	result, err := r.client.ContainerList(ctx, client.ContainerListOptions{
		Filters: make(client.Filters).Add("label", "paddock.id=" + id),
		All: true,
	})
	
	if err != nil {
		log.Err(err).Msg("failed to get sandbox")
		return err
	}

	if len(result.Items) <= 0 {
		return fmt.Errorf("not found")
	}

	item := result.Items[0]
	
	if item.State == "stopped" {
		return model.ErrSandboxNotRunning
	}


	r.client.ContainerStop(ctx, item.ID, client.ContainerStopOptions{})

	return nil
}

func (r *DockerRunner) Start(ctx context.Context, id string) error {
	result, err := r.client.ContainerList(ctx, client.ContainerListOptions{
		Filters: make(client.Filters).Add("label", "paddock.id=" + id),
		All: true,
	})
	
	if err != nil {
		log.Err(err).Msg("failed to get sandbox")
		return err
	}

	if len(result.Items) <= 0 {
		return model.ErrSandboxNotFound
	}

	item := result.Items[0]

	r.client.ContainerStart(ctx, item.ID, client.ContainerStartOptions{})

	return nil
}

func (r *DockerRunner) Exec(ctx context.Context, id string, cmd []string ) (string, string, int, error) {
	result, err := r.client.ContainerList(ctx, client.ContainerListOptions{
		Filters: make(client.Filters).Add("label", "paddock.id=" + id),
		All: true,
	})

	if err != nil {
		log.Err(err).Msg("failed to get sandbox")
		return "", "", 1, err
	}

	if len(result.Items) <= 0 {
		return "", "", 1, model.ErrSandboxNotFound
	}

	if result.Items[0].State != "running" {
		return "", "", 1, model.ErrSandboxNotRunning
	}
	
	exec, err := r.client.ExecCreate(ctx,  result.Items[0].ID ,client.ExecCreateOptions{
		Cmd: cmd,	
		TTY:  false,
		AttachStdin: true,
		AttachStderr: true ,
		AttachStdout: true,
	})

	if err != nil {
		log.Err(err).Msg("failed to create exec process")
		return "", "", 1, err
	}

	attached, err := r.client.ExecAttach(ctx, exec.ID, client.ExecAttachOptions{
		TTY: false,
	})

	if err != nil {
		log.Err(err).Msg("failed to start exec process")
		return "", "", 1, err
	}

	defer attached.Close() 
		
	var stdOut, stdErr bytes.Buffer

	stdcopy.StdCopy(&stdOut, &stdErr, attached.Reader)

	info, err := r.client.ExecInspect(ctx, exec.ID, client.ExecInspectOptions{})
	if err != nil {
		log.Err(err).Msg("failed to get exec's exit code")
		return "", "", 1, err
	}

	return stdOut.String(), stdErr.String(), info.ExitCode, nil
} 


func (r *DockerRunner) Log(ctx context.Context, id string, tail int) (string, error) {
	result, err := r.client.ContainerList(ctx, client.ContainerListOptions{
		Filters: make(client.Filters).Add("label", "paddock.id=" + id),
		All: true,
	})

	if err != nil {
		log.Err(err).Msg("failed to get sandbox")
		return "", err
	}

	if len(result.Items) <= 0 {
		return "", model.ErrSandboxNotFound
	}

	logs, err  := r.client.ContainerLogs(ctx, result.Items[0].ID, client.ContainerLogsOptions{
		ShowStdout: true,
		ShowStderr:  true,
		Tail: strconv.Itoa(tail),	
		Timestamps: true,
	})

	
	if err != nil {
		log.Err(err).Msg("failed to get err")
		return "",err
	}
	
	defer logs.Close()
	
	var stdOut, stdErr bytes.Buffer
	
	stdcopy.StdCopy(&stdOut, &stdErr, logs)

	return stdOut.String() + stdErr.String(), nil
}



func hostPort(ports network.PortMap, p network.Port) string{
	bindings, ok := ports[p]
	if !ok || len(bindings) <= 0{
		return ""
	}
	return bindings[0].HostPort
}