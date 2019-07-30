package api

import (
	"fmt"
	"time"
)

// ServiceCheck represents the consul health check that Nomad registers.
type ServiceCheck struct {
	//FIXME Id is unused. Remove?
	Id            string
	Name          string
	Type          string
	Command       string
	Args          []string
	Path          string
	Protocol      string
	PortLabel     string `mapstructure:"port"`
	AddressMode   string `mapstructure:"address_mode"`
	Interval      time.Duration
	Timeout       time.Duration
	InitialStatus string `mapstructure:"initial_status"`
	TLSSkipVerify bool   `mapstructure:"tls_skip_verify"`
	Header        map[string][]string
	Method        string
	CheckRestart  *CheckRestart `mapstructure:"check_restart"`
	GRPCService   string        `mapstructure:"grpc_service"`
	GRPCUseTLS    bool          `mapstructure:"grpc_use_tls"`
}

// Service represents a Consul service definition.
type Service struct {
	//FIXME Id is unused. Remove?
	Id           string
	Name         string
	Tags         []string
	CanaryTags   []string `mapstructure:"canary_tags"`
	PortLabel    string   `mapstructure:"port"`
	AddressMode  string   `mapstructure:"address_mode"`
	Checks       []ServiceCheck
	CheckRestart *CheckRestart `mapstructure:"check_restart"`
	Connect      *ConsulConnect
}

// Canonicalize the Service by ensuring its name and address mode are set. Task
// will be nil for group services.
func (s *Service) Canonicalize(t *Task, tg *TaskGroup, job *Job) {
	if s.Name == "" {
		if t != nil {
			s.Name = fmt.Sprintf("%s-%s-%s", *job.Name, *tg.Name, t.Name)
		} else {
			s.Name = fmt.Sprintf("%s-%s", *job.Name, *tg.Name)
		}
	}

	// Default to AddressModeAuto
	if s.AddressMode == "" {
		s.AddressMode = "auto"
	}

	// Canonicalize CheckRestart on Checks and merge Service.CheckRestart
	// into each check.
	for i, check := range s.Checks {
		s.Checks[i].CheckRestart = s.CheckRestart.Merge(check.CheckRestart)
		s.Checks[i].CheckRestart.Canonicalize()
	}
}

// ConsulConnect represents a Consul Connect jobspec stanza.
type ConsulConnect struct {
	Native         bool
	SidecarService *ConsulSidecarService `mapstructure:"sidecar_service"`
}

// ConsulSidecarService represents a Consul Connect SidecarService jobspec
// stanza.
type ConsulSidecarService struct {
	Port  string
	Proxy *ConsulProxy
}

// ConsulProxy represents a Consul Connect sidecar proxy jobspec stanza.
type ConsulProxy struct {
	Upstreams []*ConsulUpstream
	Config    map[string]interface{}
}

// ConsulUpstream represents a Consul Connect upstream jobspec stanza.
type ConsulUpstream struct {
	DestinationName string `mapstructure:"destination_name"`
	LocalBindPort   int    `mapstructure:"local_bind_port"`
}
