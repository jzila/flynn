package utils

import (
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strings"

	ct "github.com/flynn/flynn/controller/types"
	"github.com/flynn/flynn/host/types"
	"github.com/flynn/flynn/pkg/cluster"
)

func JobConfig(f *ct.ExpandedFormation, name, hostID string) *host.Job {
	t := f.Release.Processes[name]
	env := make(map[string]string, len(f.Release.Env)+len(t.Env)+4)
	for k, v := range f.Release.Env {
		env[k] = v
	}
	for k, v := range t.Env {
		env[k] = v
	}
	id := cluster.GenerateJobID(hostID)
	env["FLYNN_APP_ID"] = f.App.ID
	env["FLYNN_RELEASE_ID"] = f.Release.ID
	env["FLYNN_PROCESS_TYPE"] = name
	env["FLYNN_JOB_ID"] = id
	job := &host.Job{
		ID: id,
		Metadata: map[string]string{
			"flynn-controller.app":      f.App.ID,
			"flynn-controller.app_name": f.App.Name,
			"flynn-controller.release":  f.Release.ID,
			"flynn-controller.type":     name,
		},
		Artifact: host.Artifact{
			Type: f.Artifact.Type,
			URI:  f.Artifact.URI,
		},
		Config: host.ContainerConfig{
			Cmd:         t.Cmd,
			Env:         env,
			HostNetwork: t.HostNetwork,
		},
		Resurrect: t.Resurrect,
		Resources: t.Resources,
	}
	if len(t.Entrypoint) > 0 {
		job.Config.Entrypoint = t.Entrypoint
	}
	job.Config.Ports = make([]host.Port, len(t.Ports))
	for i, p := range t.Ports {
		job.Config.Ports[i].Proto = p.Proto
		job.Config.Ports[i].Port = p.Port
		job.Config.Ports[i].Service = p.Service
	}
	return job
}

func ProvisionVolume(h *cluster.Host, job *host.Job) error {
	vol, err := h.CreateVolume("default")
	if err != nil {
		return err
	}
	job.Config.Volumes = []host.VolumeBinding{{
		Target:    "/data",
		VolumeID:  vol.ID,
		Writeable: true,
	}}
	return nil
}

type HostClient interface {
	ID() string
	AddJob(*host.Job) error
	Attach(*host.AttachReq, bool) (cluster.AttachClient, error)
	StopJob(string) error
}

var AppNamePattern = regexp.MustCompile(`^[a-z\d]+(-[a-z\d]+)*$`)

func ParseBasicAuth(h http.Header) (username, password string, err error) {
	s := strings.SplitN(h.Get("Authorization"), " ", 2)

	if len(s) != 2 {
		return "", "", errors.New("failed to parse authentication string")
	}
	if s[0] != "Basic" {
		return "", "", fmt.Errorf("authorization scheme is %v, not Basic", s[0])
	}

	c, err := base64.StdEncoding.DecodeString(s[1])
	if err != nil {
		return "", "", errors.New("failed to parse base64 basic credentials")
	}

	s = strings.SplitN(string(c), ":", 2)
	if len(s) != 2 {
		return "", "", errors.New("failed to parse basic credentials")
	}

	return s[0], s[1], nil
}
