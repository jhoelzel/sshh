package sshconnection

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"
	"unicode"

	"shh-h/internal/apperror"
	"shh-h/internal/domain/filetransfer"
	"shh-h/internal/domain/profile"
	sshconnectiondomain "shh-h/internal/domain/sshconnection"
	"shh-h/internal/port"
	filetransferusecase "shh-h/internal/usecase/filetransfer"
	profileusecase "shh-h/internal/usecase/profile"
	sessionusecase "shh-h/internal/usecase/session"
)

type TrustStore interface {
	Probe(context.Context, string, profile.Profile) (sshconnectiondomain.HostKeyInfo, error)
	Trust(leaseID, challengeID string, permanent bool) error
}

type AuthenticationInspector interface {
	InspectAuthentication(profile.Profile) (sshconnectiondomain.AuthenticationInfo, error)
}

type Service struct {
	profiles  *profileusecase.Service
	sessions  *sessionusecase.Manager
	trust     TrustStore
	inspector AuthenticationInspector
	files     *filetransferusecase.Manager
}

func NewService(profiles *profileusecase.Service, sessions *sessionusecase.Manager, files *filetransferusecase.Manager, trust TrustStore, inspector AuthenticationInspector) *Service {
	return &Service{profiles: profiles, sessions: sessions, files: files, trust: trust, inspector: inspector}
}

func (s *Service) OpenFiles(ctx context.Context, leaseID, profileID string, credentials port.SSHCredentials) (filetransfer.Session, error) {
	selected, err := s.sshProfile(profileID)
	if err != nil {
		return filetransfer.Session{}, err
	}
	if s.files == nil {
		return filetransfer.Session{}, apperror.New(apperror.CodeUnavailable, "SFTP support is unavailable.")
	}
	return s.files.Open(ctx, leaseID, selected, credentials)
}

func (s *Service) ProbeHostKey(ctx context.Context, leaseID, profileID string) (sshconnectiondomain.HostKeyInfo, error) {
	selected, err := s.sshProfile(profileID)
	if err != nil {
		return sshconnectiondomain.HostKeyInfo{}, err
	}
	return s.trust.Probe(ctx, leaseID, selected)
}

func (s *Service) TrustHostKey(leaseID, challengeID string, permanent bool) error {
	return s.trust.Trust(leaseID, challengeID, permanent)
}

func (s *Service) InspectAuthentication(profileID string) (sshconnectiondomain.AuthenticationInfo, error) {
	selected, err := s.sshProfile(profileID)
	if err != nil {
		return sshconnectiondomain.AuthenticationInfo{}, err
	}
	return s.inspector.InspectAuthentication(selected)
}

func (s *Service) Open(ctx context.Context, leaseID, profileID string, columns, rows uint16, credentials port.SSHCredentials) (sessionusecase.Session, error) {
	selected, err := s.sshProfile(profileID)
	if err != nil {
		return sessionusecase.Session{}, err
	}
	return s.sessions.OpenSSH(ctx, leaseID, selected, columns, rows, credentials)
}

func (s *Service) ProbeQuickHostKey(ctx context.Context, leaseID string, candidate profile.Profile) (profile.Profile, sshconnectiondomain.HostKeyInfo, error) {
	selected, err := normalizeQuickProfile(candidate)
	if err != nil {
		return profile.Profile{}, sshconnectiondomain.HostKeyInfo{}, err
	}
	if s.trust == nil {
		return profile.Profile{}, sshconnectiondomain.HostKeyInfo{}, apperror.New(
			apperror.CodeUnavailable, "SSH host-key support is unavailable.",
		)
	}
	info, err := s.trust.Probe(ctx, leaseID, selected)
	if err != nil {
		return profile.Profile{}, sshconnectiondomain.HostKeyInfo{}, err
	}
	return selected, info, nil
}

func (s *Service) InspectQuickAuthentication(candidate profile.Profile) (sshconnectiondomain.AuthenticationInfo, error) {
	selected, err := normalizeQuickProfile(candidate)
	if err != nil {
		return sshconnectiondomain.AuthenticationInfo{}, err
	}
	if s.inspector == nil {
		return sshconnectiondomain.AuthenticationInfo{}, apperror.New(
			apperror.CodeUnavailable, "SSH authentication support is unavailable.",
		)
	}
	return s.inspector.InspectAuthentication(selected)
}

func (s *Service) OpenQuick(ctx context.Context, leaseID string, candidate profile.Profile, columns, rows uint16, credentials port.SSHCredentials) (sessionusecase.Session, error) {
	selected, err := normalizeQuickProfile(candidate)
	if err != nil {
		return sessionusecase.Session{}, err
	}
	if s.sessions == nil {
		return sessionusecase.Session{}, apperror.New(apperror.CodeUnavailable, "SSH terminal support is unavailable.")
	}
	return s.sessions.OpenSSH(ctx, leaseID, selected, columns, rows, credentials)
}

func (s *Service) sshProfile(profileID string) (profile.Profile, error) {
	selected, found := s.profiles.Find(profileID)
	if !found {
		return profile.Profile{}, apperror.New(apperror.CodeNotFound, "Profile was not found.")
	}
	if selected.Protocol != profile.ProtocolSSH {
		return profile.Profile{}, apperror.New(apperror.CodeInvalidArgument, "Profile is not an SSH connection.")
	}
	return selected, nil
}

func normalizeQuickProfile(candidate profile.Profile) (profile.Profile, error) {
	host := strings.TrimSpace(candidate.Host)
	if strings.HasPrefix(host, "[") && strings.HasSuffix(host, "]") {
		host = strings.TrimSpace(host[1 : len(host)-1])
	}
	selected := profile.Profile{
		Protocol: profile.ProtocolSSH, Host: host, Port: candidate.Port,
		Username: strings.TrimSpace(candidate.Username), Authentication: candidate.Authentication,
		IdentityFile: strings.TrimSpace(candidate.IdentityFile),
	}.WithDefaults(time.Now().UTC())
	if err := validateQuickHost(selected.Host); err != nil {
		return profile.Profile{}, err
	}
	if err := validateQuickText("username", selected.Username, 255, false); err != nil {
		return profile.Profile{}, err
	}
	if err := validateQuickText("identity file", selected.IdentityFile, 4096, true); err != nil {
		return profile.Profile{}, err
	}

	selected.Name = selected.Endpoint()
	digest := sha256.Sum256([]byte(strings.Join([]string{
		selected.Host, strconv.Itoa(selected.Port), selected.Username,
		string(selected.Authentication), selected.IdentityFile,
	}, "\x00")))
	selected.ID = "quick-ssh-" + hex.EncodeToString(digest[:8])
	if err := selected.Validate(); err != nil {
		return profile.Profile{}, apperror.Wrap(
			apperror.CodeInvalidArgument, "validate quick SSH connection", err.Error(), err,
		)
	}
	return selected, nil
}

func validateQuickHost(host string) error {
	if host == "" {
		return apperror.New(apperror.CodeInvalidArgument, "Host is required.")
	}
	if len(host) > 255 {
		return apperror.New(apperror.CodeInvalidArgument, "Host exceeds 255 characters.")
	}
	for _, character := range host {
		if unicode.IsControl(character) || unicode.IsSpace(character) || strings.ContainsRune("/@[]", character) {
			return apperror.New(
				apperror.CodeInvalidArgument, "Host must be a hostname or IP address without a scheme, port, or path.",
			)
		}
	}
	if strings.Contains(host, ":") && !isIPv6Literal(host) {
		return apperror.New(apperror.CodeInvalidArgument, "Host must not include a port; use the port field.")
	}
	return nil
}

func isIPv6Literal(host string) bool {
	address := host
	if zoneIndex := strings.LastIndexByte(address, '%'); zoneIndex >= 0 {
		if zoneIndex == len(address)-1 {
			return false
		}
		address = address[:zoneIndex]
	}
	return net.ParseIP(address) != nil
}

func validateQuickText(label, value string, limit int, allowSpace bool) error {
	if len(value) > limit {
		return apperror.New(
			apperror.CodeInvalidArgument, fmt.Sprintf("%s exceeds %d characters.", label, limit),
		)
	}
	for _, character := range value {
		if unicode.IsControl(character) || (!allowSpace && unicode.IsSpace(character)) {
			return apperror.New(
				apperror.CodeInvalidArgument,
				fmt.Sprintf("%s contains unsupported whitespace or control characters.", label),
			)
		}
	}
	return nil
}
