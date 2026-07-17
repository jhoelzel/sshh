package tunnel

import (
	"errors"
	"fmt"
	"net"
	"strings"
	"time"
)

type Kind string

const (
	KindLocal   Kind = "local"
	KindRemote  Kind = "remote"
	KindDynamic Kind = "dynamic"
)

type State string

const (
	StateStarting State = "starting"
	StateActive   State = "active"
	StateRetrying State = "retrying"
	StateFailed   State = "failed"
	StateStopped  State = "stopped"
)

type Config struct {
	ID              string    `json:"id"`
	Name            string    `json:"name"`
	ProfileID       string    `json:"profileId"`
	Kind            Kind      `json:"kind"`
	BindAddress     string    `json:"bindAddress"`
	BindPort        int       `json:"bindPort"`
	DestinationHost string    `json:"destinationHost"`
	DestinationPort int       `json:"destinationPort"`
	AutoStart       bool      `json:"autoStart"`
	Reconnect       bool      `json:"reconnect"`
	CreatedAt       time.Time `json:"createdAt"`
	UpdatedAt       time.Time `json:"updatedAt"`
}

func (c Config) WithDefaults(now time.Time) Config {
	c.ID = strings.TrimSpace(c.ID)
	c.Name = strings.TrimSpace(c.Name)
	c.ProfileID = strings.TrimSpace(c.ProfileID)
	c.BindAddress = strings.TrimSpace(c.BindAddress)
	c.DestinationHost = strings.TrimSpace(c.DestinationHost)
	if c.Kind == "" {
		c.Kind = KindLocal
	}
	if c.BindAddress == "" {
		c.BindAddress = "127.0.0.1"
	}
	if c.CreatedAt.IsZero() {
		c.CreatedAt = now
	}
	if c.UpdatedAt.IsZero() {
		c.UpdatedAt = now
	}
	return c
}

func (c Config) Validate() error {
	if c.ID == "" {
		return errors.New("id is required")
	}
	if c.Name == "" {
		return errors.New("name is required")
	}
	if c.ProfileID == "" {
		return errors.New("ssh profile is required")
	}
	if net.ParseIP(strings.Trim(c.BindAddress, "[]")) == nil {
		return errors.New("bind address must be an IPv4 or IPv6 address")
	}
	if c.BindPort < 0 || c.BindPort > 65535 {
		return errors.New("bind port must be between 0 and 65535")
	}
	switch c.Kind {
	case KindLocal, KindRemote:
		if c.DestinationHost == "" {
			return errors.New("destination host is required")
		}
		if c.DestinationPort < 1 || c.DestinationPort > 65535 {
			return errors.New("destination port must be between 1 and 65535")
		}
	case KindDynamic:
		if c.DestinationHost != "" || c.DestinationPort != 0 {
			return errors.New("dynamic tunnels do not use a fixed destination")
		}
	default:
		return fmt.Errorf("unsupported tunnel kind %q", c.Kind)
	}
	return nil
}

func (c Config) BindsAllInterfaces() bool {
	address := net.ParseIP(strings.Trim(c.BindAddress, "[]"))
	return address != nil && address.IsUnspecified()
}

type Snapshot struct {
	ConfigID     string `json:"configId"`
	LeaseID      string `json:"leaseId"`
	State        State  `json:"state"`
	BoundAddress string `json:"boundAddress"`
	Message      string `json:"message"`
	StartedAt    string `json:"startedAt"`
	UpdatedAt    string `json:"updatedAt"`
}

func (s Snapshot) Live() bool {
	return s.State == StateStarting || s.State == StateActive || s.State == StateRetrying
}
