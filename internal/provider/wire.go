package provider

import "github.com/sasiruLK/tinycloud-platform/internal/infra"

// The response bodies of the `/v0` contract, exactly as provider-contract-v0.yaml
// describes them. The Provider server marshals these and the client unmarshals
// them, so the two halves of an in-tree Provider cannot disagree about the wire
// even by accident.
//
// The interesting types inside — Instance, Series, Alarm, BackupObject — are
// the domain types from internal/infra, unchanged. This contract introduces no
// vocabulary of its own.

// capabilitiesBody answers GET /v0/capabilities.
type capabilitiesBody struct {
	Kind         string   `json:"kind"`
	Provider     string   `json:"provider,omitempty"`
	Capabilities []string `json:"capabilities"`
}

// instancesBody answers GET /v0/infra/instances.
type instancesBody struct {
	Instances []infra.InstanceInfo `json:"instances"`
}

// seriesBody answers GET /v0/infra/metrics.
type seriesBody struct {
	Series []infra.Series `json:"series"`
}

// alarmsBody answers GET /v0/infra/alarms.
type alarmsBody struct {
	Alarms []infra.AlarmStatus `json:"alarms"`
}

// ingressBody answers GET /v0/infra/ingress. An address still being assigned
// is the empty string, not an absent field.
type ingressBody struct {
	PublicIP string `json:"publicIp"`
}

// backupsBody answers GET /v0/infra/backups.
type backupsBody struct {
	Objects []infra.ObjectInfo `json:"objects"`
}

// errorBody is every non-2xx response.
type errorBody struct {
	Error   string `json:"error"`
	Message string `json:"message"`
}

// The machine-readable codes of errorBody.
const (
	codeBadRequest     = "bad_request"
	codeUnauthorized   = "unauthorized"
	codeNotImplemented = "not_implemented"
	codeUpstreamError  = "upstream_error"
)

// Contract paths. The client builds requests from these and the server
// registers them, so a typo cannot make the two halves miss each other.
const (
	pathHealth       = "/healthz"
	pathCapabilities = "/" + ContractVersion + "/capabilities"
	pathInstances    = "/" + ContractVersion + "/infra/instances"
	pathMetrics      = "/" + ContractVersion + "/infra/metrics"
	pathAlarms       = "/" + ContractVersion + "/infra/alarms"
	pathIngress      = "/" + ContractVersion + "/infra/ingress"
	pathBackups      = "/" + ContractVersion + "/infra/backups"
)

// pathFor maps a Capability to the path serving it.
func pathFor(capability string) string {
	switch capability {
	case CapabilityInstances:
		return pathInstances
	case CapabilityMetrics:
		return pathMetrics
	case CapabilityAlarms:
		return pathAlarms
	case CapabilityIngress:
		return pathIngress
	case CapabilityBackups:
		return pathBackups
	}
	return ""
}
