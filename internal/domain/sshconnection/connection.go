package sshconnection

type HostKeyStatus string

const (
	HostKeyKnown   HostKeyStatus = "known"
	HostKeyUnknown HostKeyStatus = "unknown"
	HostKeyChanged HostKeyStatus = "changed"
)

type HostKeyInfo struct {
	Status      HostKeyStatus
	Host        string
	Address     string
	Algorithm   string
	Fingerprint string
	ChallengeID string
}

type SecretRequirement string

const (
	SecretNone       SecretRequirement = "none"
	SecretPassword   SecretRequirement = "password"
	SecretPassphrase SecretRequirement = "passphrase"
)

type AuthenticationInfo struct {
	Secret       SecretRequirement
	IdentityFile string
}
