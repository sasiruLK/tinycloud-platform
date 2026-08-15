package manifests

import (
	"fmt"
	"regexp"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateCreateAppRequest(t *testing.T) {
	tests := []struct {
		name    string
		req     CreateAppRequest
		wantErr bool
	}{
		{
			name: "valid",
			req: CreateAppRequest{
				Name: "my-app", Image: "ghcr.io/user/my-app", Tag: "a1b2c3d4e5f60718293a4b5c6d7e8f90a1b2c3d4",
				Replicas: 2, Port: 8080,
			},
		},
		{
			name:    "invalid name",
			req:     CreateAppRequest{Name: "My_App", Image: "ghcr.io/user/app", Tag: "a1b2c3d4e5f60718293a4b5c6d7e8f90a1b2c3d4", Replicas: 1, Port: 8080},
			wantErr: true,
		},
		{
			name:    "image with tag",
			req:     CreateAppRequest{Name: "my-app", Image: "ghcr.io/user/app:1.0.0", Tag: "a1b2c3d4e5f60718293a4b5c6d7e8f90a1b2c3d4", Replicas: 1, Port: 8080},
			wantErr: true,
		},
		{
			name:    "invalid tag: not a commit SHA",
			req:     CreateAppRequest{Name: "my-app", Image: "ghcr.io/user/app", Tag: "not-a-sha", Replicas: 1, Port: 8080},
			wantErr: true,
		},
		{
			name:    "replicas too high",
			req:     CreateAppRequest{Name: "my-app", Image: "ghcr.io/user/app", Tag: "a1b2c3d4e5f60718293a4b5c6d7e8f90a1b2c3d4", Replicas: 11, Port: 8080},
			wantErr: true,
		},
		{
			name:    "reserved name",
			req:     CreateAppRequest{Name: "tinycloud-api", Image: "ghcr.io/user/app", Tag: "a1b2c3d4e5f60718293a4b5c6d7e8f90a1b2c3d4", Replicas: 1, Port: 8080},
			wantErr: true,
		},
		{
			name:    "non-standard port",
			req:     CreateAppRequest{Name: "my-app", Image: "ghcr.io/user/app", Tag: "a1b2c3d4e5f60718293a4b5c6d7e8f90a1b2c3d4", Replicas: 1, Port: 3000},
			wantErr: true,
		},
		{
			name: "fixed PORT env is allowed",
			req: CreateAppRequest{
				Name: "my-app", Image: "ghcr.io/user/app", Tag: "a1b2c3d4e5f60718293a4b5c6d7e8f90a1b2c3d4",
				Replicas: 1, Port: 8080, Env: map[string]string{"PORT": "8080"},
			},
		},
		{
			name: "mismatched PORT env is rejected",
			req: CreateAppRequest{
				Name: "my-app", Image: "ghcr.io/user/app", Tag: "a1b2c3d4e5f60718293a4b5c6d7e8f90a1b2c3d4",
				Replicas: 1, Port: 8080, Env: map[string]string{"PORT": "3000"},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateCreateAppRequest(&tt.req)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestGenerateAppFiles(t *testing.T) {
	req := CreateAppRequest{
		Name: "demo-app", Image: "ghcr.io/user/demo", Tag: "b2c3d4e5f60718293a4b5c6d7e8f90a1b2c3d4e5",
		Replicas: 3, Port: 8080,
		Env: map[string]string{"LOG_LEVEL": "debug"},
	}

	files := GenerateAppFiles(req)
	require.Len(t, files, 8)

	deployment := string(files["apps/demo-app/deployment.yaml"])
	assert.Contains(t, deployment, "name: demo-app")
	assert.Contains(t, deployment, "replicas: 3")
	assert.Contains(t, deployment, "containerPort: 8080")
	assert.Contains(t, deployment, "name: PORT")
	assert.Contains(t, deployment, `value: "8080"`)
	assert.Contains(t, deployment, "tcpSocket")
	assert.Contains(t, deployment, "ghcr.io/user/demo:b2c3d4e5f60718293a4b5c6d7e8f90a1b2c3d4e5")
	assert.Contains(t, deployment, "ghcr-creds")
	assert.Contains(t, deployment, `name: LOG_LEVEL`)
	assert.Contains(t, deployment, `value: "debug"`)

	np := string(files["apps/demo-app/network-policies.yaml"])
	assert.Contains(t, np, "port: 8080")
	assert.Contains(t, np, "k8s-app: kube-dns")

	service := string(files["apps/demo-app/service.yaml"])
	assert.Contains(t, service, "port: 80")

	kustomize := string(files["apps/demo-app/kustomization.yaml"])
	assert.Contains(t, kustomize, "namespace: demo-app")
	assert.Contains(t, kustomize, "newTag: b2c3d4e5f60718293a4b5c6d7e8f90a1b2c3d4e5")

	sync := string(files["apps/demo-app/pull-secret-sync.yaml"])
	assert.Contains(t, sync, "sync-ghcr-creds")
	assert.Contains(t, sync, "demo-app-ghcr-creds-reader")
	assert.Contains(t, sync, "kind: ClusterRole")
	assert.Contains(t, sync, "kind: ClusterRoleBinding")
	assert.Contains(t, sync, "resourceNames: [\"ghcr-creds\"]")
	assert.Contains(t, sync, "argocd.argoproj.io/hook: PreSync")
	assert.Contains(t, sync, `sync-wave: "-3"`)
	assert.Contains(t, sync, "sync-wave: \"-1\"")

	updater := string(files["argocd/imageupdater-demo-app.yaml"])
	assert.Contains(t, updater, "name: demo-app")
	assert.Contains(t, updater, "imageName: ghcr.io/user/demo")
}

func TestAppBaseURL(t *testing.T) {
	assert.Equal(t, "https://demo-app.sasiru.lk/", AppBaseURL("demo-app"))
}

// TestTagContractMatchesImageUpdater guards the invariant that broke automated
// updates for onboarded apps: ValidateCreateAppRequest required semver while the
// generated ImageUpdater only allowed 40-character commit SHAs. The two sets were
// disjoint, so no onboarded app could ever receive an automated image update.
//
// Any tag the API accepts MUST be matchable by the allowTags regexp that ships in
// the generated ImageUpdater.
func TestTagContractMatchesImageUpdater(t *testing.T) {
	const tag = "b2c3d4e5f60718293a4b5c6d7e8f90a1b2c3d4e5"

	req := CreateAppRequest{
		Name: "contract-app", Image: "ghcr.io/user/contract-app", Tag: tag,
		Replicas: 1, Port: 8080,
	}
	NormalizeCreateAppRequest(&req)
	require.NoError(t, ValidateCreateAppRequest(&req), "API must accept a commit SHA tag")

	updater := string(GenerateAppFiles(req)["argocd/imageupdater-contract-app.yaml"])
	require.NotEmpty(t, updater, "generator must emit an ImageUpdater")

	// Pull the regexp out of `allowTags: "regexp:^...$"` and apply it to the tag.
	m := regexp.MustCompile(`allowTags:\s*"regexp:([^"]+)"`).FindStringSubmatch(updater)
	require.Len(t, m, 2, "ImageUpdater must declare an allowTags regexp")

	allow, err := regexp.Compile(m[1])
	require.NoError(t, err, "allowTags regexp must compile")
	assert.True(t, allow.MatchString(tag),
		"tag %q is accepted by the API but rejected by allowTags %q; automated updates would never fire", tag, m[1])
}

// PlaceholderTag exists so callers can validate an app that has not been built
// yet. If it stops satisfying the tag rule, every build request 400s — which is
// exactly what happened when tag validation moved from semver to commit SHAs.
func TestPlaceholderTagPassesValidation(t *testing.T) {
	req := CreateAppRequest{
		Name: "placeholder-app", Image: "ghcr.io/user/placeholder-app",
		Tag: PlaceholderTag, Replicas: 1, Port: 8080,
	}
	NormalizeCreateAppRequest(&req)
	require.NoError(t, ValidateCreateAppRequest(&req),
		"PlaceholderTag must satisfy the tag rule or build requests cannot be validated")
}

// Probes must not require an application-level contract. The platform deploys
// arbitrary repositories; when probes hit /healthz, any app that did not serve
// that path crash-looped forever while answering real traffic correctly.
func TestProbesDoNotRequireHealthEndpoint(t *testing.T) {
	req := CreateAppRequest{
		Name: "probe-app", Image: "ghcr.io/user/probe-app",
		Tag: PlaceholderTag, Replicas: 1, Port: 8080,
	}
	NormalizeCreateAppRequest(&req)
	deployment := string(GenerateAppFiles(req)["apps/probe-app/deployment.yaml"])

	require.NotEmpty(t, deployment)
	assert.NotContains(t, deployment, "httpGet",
		"probes must not depend on the app serving a specific HTTP path")
	assert.Contains(t, deployment, "tcpSocket")
}

// The namespace quota must be able to hold maxReplicas pods at the deployment's
// own requests and limits. Previously an app could pass validation at 10
// replicas and then be refused by its own quota at 6.
func TestQuotaHoldsMaxReplicas(t *testing.T) {
	req := CreateAppRequest{
		Name: "quota-app", Image: "ghcr.io/user/quota-app",
		Tag: PlaceholderTag, Replicas: maxReplicas, Port: 8080,
	}
	NormalizeCreateAppRequest(&req)
	require.NoError(t, ValidateCreateAppRequest(&req),
		"maxReplicas must itself be a valid replica count")

	files := GenerateAppFiles(req)
	quota := string(files["apps/quota-app/resource-quota.yaml"])
	deployment := string(files["apps/quota-app/deployment.yaml"])

	// Pod count in the quota must not be below what validation permits.
	assert.Contains(t, quota, fmt.Sprintf("pods: %q", fmt.Sprintf("%d", maxReplicas)))

	// Requests must be tight enough that the scheduler is not misled: a request
	// far below the limit lets it overcommit a node that looks nearly empty.
	assert.Contains(t, deployment, "memory: 128Mi")
	assert.Contains(t, deployment, "memory: 256Mi")
}
