package manifests

import (
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
				Name: "my-app", Image: "ghcr.io/user/my-app", Tag: "1.0.0",
				Replicas: 2, Port: 8080,
			},
		},
		{
			name:    "invalid name",
			req:     CreateAppRequest{Name: "My_App", Image: "ghcr.io/user/app", Tag: "1.0.0", Replicas: 1, Port: 8080},
			wantErr: true,
		},
		{
			name:    "image with tag",
			req:     CreateAppRequest{Name: "my-app", Image: "ghcr.io/user/app:1.0.0", Tag: "1.0.0", Replicas: 1, Port: 8080},
			wantErr: true,
		},
		{
			name:    "invalid semver",
			req:     CreateAppRequest{Name: "my-app", Image: "ghcr.io/user/app", Tag: "latest", Replicas: 1, Port: 8080},
			wantErr: true,
		},
		{
			name:    "replicas too high",
			req:     CreateAppRequest{Name: "my-app", Image: "ghcr.io/user/app", Tag: "1.0.0", Replicas: 11, Port: 8080},
			wantErr: true,
		},
		{
			name:    "reserved name",
			req:     CreateAppRequest{Name: "tinycloud-api", Image: "ghcr.io/user/app", Tag: "1.0.0", Replicas: 1, Port: 8080},
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
		Name: "demo-app", Image: "ghcr.io/user/demo", Tag: "2.1.0",
		Replicas: 3, Port: 3000,
		Env: map[string]string{"LOG_LEVEL": "debug"},
	}

	files := GenerateAppFiles(req)
	require.Len(t, files, 8)

	deployment := string(files["apps/demo-app/deployment.yaml"])
	assert.Contains(t, deployment, "name: demo-app")
	assert.Contains(t, deployment, "replicas: 3")
	assert.Contains(t, deployment, "containerPort: 3000")
	assert.Contains(t, deployment, "ghcr.io/user/demo:2.1.0")
	assert.Contains(t, deployment, "ocir-creds")
	assert.Contains(t, deployment, `name: LOG_LEVEL`)
	assert.Contains(t, deployment, `value: "debug"`)

	np := string(files["apps/demo-app/network-policies.yaml"])
	assert.Contains(t, np, "port: 3000")
	assert.Contains(t, np, "k8s-app: kube-dns")

	kustomize := string(files["apps/demo-app/kustomization.yaml"])
	assert.Contains(t, kustomize, "namespace: demo-app")
	assert.Contains(t, kustomize, "newTag: 2.1.0")

	sync := string(files["apps/demo-app/pull-secret-sync.yaml"])
	assert.Contains(t, sync, "sync-ocir-creds")
	assert.Contains(t, sync, "demo-app-ocir-creds-reader")
	assert.Contains(t, sync, "kind: ClusterRole")
	assert.Contains(t, sync, "kind: ClusterRoleBinding")
	assert.Contains(t, sync, "resourceNames: [\"ocir-creds\"]")
	assert.Contains(t, sync, "argocd.argoproj.io/hook: PreSync")
	assert.Contains(t, sync, `sync-wave: "-3"`)
	assert.Contains(t, sync, "sync-wave: \"-1\"")

	updater := string(files["argocd/imageupdater-demo-app.yaml"])
	assert.Contains(t, updater, "name: demo-app")
	assert.Contains(t, updater, "imageName: ghcr.io/user/demo")
}
