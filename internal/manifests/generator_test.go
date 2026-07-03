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
				Name: "my-app", Image: "iad.ocir.io/user/tinycloud/my-app", Tag: "1.0.0",
				Replicas: 2, Port: 8080,
			},
		},
		{
			name:    "invalid name",
			req:     CreateAppRequest{Name: "My_App", Image: "iad.ocir.io/user/tinycloud/app", Tag: "1.0.0", Replicas: 1, Port: 8080},
			wantErr: true,
		},
		{
			name:    "image with tag",
			req:     CreateAppRequest{Name: "my-app", Image: "iad.ocir.io/user/tinycloud/app:1.0.0", Tag: "1.0.0", Replicas: 1, Port: 8080},
			wantErr: true,
		},
		{
			name:    "invalid semver",
			req:     CreateAppRequest{Name: "my-app", Image: "iad.ocir.io/user/tinycloud/app", Tag: "latest", Replicas: 1, Port: 8080},
			wantErr: true,
		},
		{
			name:    "replicas too high",
			req:     CreateAppRequest{Name: "my-app", Image: "iad.ocir.io/user/tinycloud/app", Tag: "1.0.0", Replicas: 11, Port: 8080},
			wantErr: true,
		},
		{
			name:    "reserved name",
			req:     CreateAppRequest{Name: "tinycloud-api", Image: "iad.ocir.io/user/tinycloud/app", Tag: "1.0.0", Replicas: 1, Port: 8080},
			wantErr: true,
		},
		{
			name:    "non-standard port",
			req:     CreateAppRequest{Name: "my-app", Image: "iad.ocir.io/user/tinycloud/app", Tag: "1.0.0", Replicas: 1, Port: 3000},
			wantErr: true,
		},
		{
			name: "fixed PORT env is allowed",
			req: CreateAppRequest{
				Name: "my-app", Image: "iad.ocir.io/user/tinycloud/app", Tag: "1.0.0",
				Replicas: 1, Port: 8080, Env: map[string]string{"PORT": "8080"},
			},
		},
		{
			name: "mismatched PORT env is rejected",
			req: CreateAppRequest{
				Name: "my-app", Image: "iad.ocir.io/user/tinycloud/app", Tag: "1.0.0",
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
		Name: "demo-app", Image: "iad.ocir.io/user/tinycloud/demo", Tag: "2.1.0",
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
	assert.Contains(t, deployment, "path: /healthz")
	assert.Contains(t, deployment, "iad.ocir.io/user/tinycloud/demo:2.1.0")
	assert.Contains(t, deployment, "ocir-creds")
	assert.Contains(t, deployment, `name: LOG_LEVEL`)
	assert.Contains(t, deployment, `value: "debug"`)

	np := string(files["apps/demo-app/network-policies.yaml"])
	assert.Contains(t, np, "port: 8080")
	assert.Contains(t, np, "k8s-app: kube-dns")

	service := string(files["apps/demo-app/service.yaml"])
	assert.Contains(t, service, "port: 80")

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
	assert.Contains(t, updater, "imageName: iad.ocir.io/user/tinycloud/demo")
}

func TestAppBaseURL(t *testing.T) {
	assert.Equal(t, "https://demo-app.sasiru.lk/", AppBaseURL("demo-app"))
}
