package profile

import (
	"errors"
	"fmt"
	"net"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

type Protocol string

type Authentication string

const (
	ProtocolLocal Protocol = "local"
	ProtocolSSH   Protocol = "ssh"

	AuthenticationAuto     Authentication = "auto"
	AuthenticationAgent    Authentication = "agent"
	AuthenticationKey      Authentication = "key"
	AuthenticationPassword Authentication = "password"

	maxEnvironmentOverrides = 128
)

var portableEnvironmentName = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]{0,127}$`)

type Profile struct {
	ID               string            `json:"id"`
	Name             string            `json:"name"`
	Protocol         Protocol          `json:"protocol"`
	Host             string            `json:"host,omitempty"`
	Port             int               `json:"port,omitempty"`
	Username         string            `json:"username,omitempty"`
	Authentication   Authentication    `json:"authentication,omitempty"`
	IdentityFile     string            `json:"identityFile,omitempty"`
	Shell            string            `json:"shell,omitempty"`
	Arguments        []string          `json:"arguments,omitempty"`
	WorkingDirectory string            `json:"workingDirectory,omitempty"`
	Environment      map[string]string `json:"environment,omitempty"`
	Tags             []string          `json:"tags,omitempty"`
	Group            string            `json:"group,omitempty"`
	Favorite         bool              `json:"favorite,omitempty"`
	CreatedAt        time.Time         `json:"created_at"`
	UpdatedAt        time.Time         `json:"updated_at"`
}

func (p Profile) Validate() error {
	if strings.TrimSpace(p.ID) == "" {
		return errors.New("id is required")
	}
	if strings.TrimSpace(p.Name) == "" {
		return errors.New("name is required")
	}
	if err := validateEnvironment(p.Environment); err != nil {
		return err
	}

	switch p.Protocol {
	case ProtocolLocal:
		return nil
	case ProtocolSSH:
		if strings.TrimSpace(p.Host) == "" {
			return errors.New("host is required for ssh profiles")
		}
		if p.Port < 1 || p.Port > 65535 {
			return errors.New("port must be between 1 and 65535")
		}
		switch p.Authentication {
		case AuthenticationAuto, AuthenticationAgent, AuthenticationKey, AuthenticationPassword:
		default:
			return fmt.Errorf("unsupported ssh authentication %q", p.Authentication)
		}
		if p.Authentication == AuthenticationKey && strings.TrimSpace(p.IdentityFile) == "" {
			return errors.New("identity file is required for key authentication")
		}
		return nil
	default:
		return fmt.Errorf("unsupported protocol %q", p.Protocol)
	}
}

func validateEnvironment(environment map[string]string) error {
	if len(environment) > maxEnvironmentOverrides {
		return fmt.Errorf("environment supports at most %d overrides", maxEnvironmentOverrides)
	}

	names := make([]string, 0, len(environment))
	for name := range environment {
		names = append(names, name)
	}
	sort.Strings(names)

	canonicalNames := make(map[string]string, len(names))
	for _, name := range names {
		if !portableEnvironmentName.MatchString(name) {
			return fmt.Errorf(
				"environment variable %q must start with a letter or underscore and contain only letters, digits, and underscores",
				name,
			)
		}

		canonical := strings.ToUpper(name)
		if previous, exists := canonicalNames[canonical]; exists {
			return fmt.Errorf("environment variables %q and %q differ only by case", previous, name)
		}
		canonicalNames[canonical] = name

		switch canonical {
		case "TERM", "COLORTERM", "SHHH_SESSION_ID":
			return fmt.Errorf("environment variable %q is managed by shh-h", name)
		}
		if strings.IndexByte(environment[name], 0) >= 0 {
			return fmt.Errorf("environment variable %q contains a null byte", name)
		}
	}
	return nil
}

func (p Profile) WithDefaults(now time.Time) Profile {
	p.ID = strings.TrimSpace(p.ID)
	p.Name = strings.TrimSpace(p.Name)
	p.Host = strings.TrimSpace(p.Host)
	p.Username = strings.TrimSpace(p.Username)
	p.IdentityFile = strings.TrimSpace(p.IdentityFile)
	p.Shell = strings.TrimSpace(p.Shell)
	p.WorkingDirectory = strings.TrimSpace(p.WorkingDirectory)
	p.Group = strings.TrimSpace(p.Group)

	if p.Protocol == "" {
		p.Protocol = ProtocolLocal
	}
	if p.Protocol == ProtocolSSH && p.Port == 0 {
		p.Port = 22
	}
	if p.Protocol == ProtocolSSH && p.Authentication == "" {
		p.Authentication = AuthenticationAuto
	}
	if p.CreatedAt.IsZero() {
		p.CreatedAt = now
	}
	if p.UpdatedAt.IsZero() {
		p.UpdatedAt = now
	}

	return p
}

func (p Profile) Endpoint() string {
	switch p.Protocol {
	case ProtocolLocal:
		if p.Shell != "" {
			return p.Shell
		}
		return "Default login shell"
	case ProtocolSSH:
		host := strings.TrimPrefix(strings.TrimSuffix(p.Host, "]"), "[")
		address := net.JoinHostPort(host, strconv.Itoa(p.Port))
		if p.Username != "" {
			return fmt.Sprintf("%s@%s", p.Username, address)
		}
		return address
	default:
		return string(p.Protocol)
	}
}
