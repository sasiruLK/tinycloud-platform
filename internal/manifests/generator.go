package manifests

import (
	"fmt"
	"regexp"
	"strings"
)

const (
	maxReplicas = 10
	minReplicas = 0
)

var (
	appNameRegex = regexp.MustCompile(`^[a-z0-9]([-a-z0-9]*[a-z0-9])?$`)
	semverRegex  = regexp.MustCompile(`^v?[0-9]+\.[0-9]+\.[0-9]+(-[a-zA-Z0-9.]+)?$`)
	imageRegex   = regexp.MustCompile(`^[a-z0-9]+(?:[._-][a-z0-9]+)*(?:/[a-z0-9]+(?:[._-][a-z0-9]+)*)+$`)
	reservedNames = map[string]bool{
		"tinycloud-api": true, "tinycloud-ui": true, "tinycloud-platform": true,
		"nginx-proxy": true, "oauth2-proxy": true, "traefik": true,
		"kube-system": true, "monitoring": true, "argocd": true,
		"cert-manager": true, "kube-dns": true, "default": true,
	}
)

// CreateAppRequest is the payload for POST /v1/apps
type CreateAppRequest struct {
	Name     string            `json:"name"`
	Image    string            `json:"image"`
	Tag      string            `json:"tag"`
	Replicas int               `json:"replicas"`
	Port     int               `json:"port"`
	Env      map[string]string `json:"env,omitempty"`
}

// CreateAppResponse is returned after a successful Git commit
type CreateAppResponse struct {
	Name   string `json:"name"`
	URL    string `json:"url"`
	Repo   string `json:"repo"`
	Path   string `json:"path"`
	Status string `json:"status"`
}

// PlatformBaseURL is the external platform URL for app links
const PlatformBaseURL = "https://tinycloud-platform.duckdns.org"

// ValidateCreateAppRequest validates onboarding input
func ValidateCreateAppRequest(req *CreateAppRequest) error {
	if req == nil {
		return fmt.Errorf("request body is required")
	}

	name := strings.TrimSpace(req.Name)
	if name == "" {
		return fmt.Errorf("name is required")
	}
	if len(name) > 63 {
		return fmt.Errorf("name must be at most 63 characters")
	}
	if !appNameRegex.MatchString(name) {
		return fmt.Errorf("name must be DNS-1123 lowercase alphanumeric with hyphens")
	}
	if reservedNames[name] {
		return fmt.Errorf("name '%s' is reserved and cannot be used", name)
	}

	image := strings.TrimSpace(req.Image)
	if image == "" {
		return fmt.Errorf("image is required")
	}
	if strings.Contains(image, ":") {
		return fmt.Errorf("image must not include a tag; use the tag field")
	}
	if !imageRegex.MatchString(strings.ToLower(image)) {
		return fmt.Errorf("image must be a valid container image reference")
	}

	tag := strings.TrimSpace(req.Tag)
	if tag == "" {
		return fmt.Errorf("tag is required")
	}
	if !semverRegex.MatchString(tag) {
		return fmt.Errorf("tag must be semver (e.g. 1.0.0)")
	}

	if req.Replicas < 1 || req.Replicas > maxReplicas {
		return fmt.Errorf("replicas must be between 1 and %d", maxReplicas)
	}

	if req.Port < 1 || req.Port > 65535 {
		return fmt.Errorf("port must be between 1 and 65535")
	}

	return nil
}

// NormalizeCreateAppRequest trims and applies defaults
func NormalizeCreateAppRequest(req *CreateAppRequest) {
	req.Name = strings.TrimSpace(req.Name)
	req.Image = strings.TrimSpace(req.Image)
	req.Tag = strings.TrimSpace(req.Tag)
	if req.Port == 0 {
		req.Port = 8080
	}
	if req.Replicas == 0 {
		req.Replicas = 1
	}
}

// GenerateAppFiles returns Git paths and file contents for a new app
func GenerateAppFiles(req CreateAppRequest) map[string][]byte {
	vars := map[string]string{
		"{{APP_NAME}}": strings.TrimSpace(req.Name),
		"{{IMAGE}}":    strings.TrimSpace(req.Image),
		"{{TAG}}":      strings.TrimSpace(req.Tag),
		"{{REPLICAS}}": fmt.Sprintf("%d", req.Replicas),
		"{{PORT}}":     fmt.Sprintf("%d", req.Port),
		"{{ENV_VARS}}": renderEnvVars(req.Env),
	}

	appPath := fmt.Sprintf("apps/%s", req.Name)
	files := map[string][]byte{
		fmt.Sprintf("%s/namespace.yaml", appPath):              []byte(replaceVars(namespaceTemplate, vars)),
		fmt.Sprintf("%s/deployment.yaml", appPath):             []byte(replaceVars(deploymentTemplate, vars)),
		fmt.Sprintf("%s/service.yaml", appPath):                []byte(replaceVars(serviceTemplate, vars)),
		fmt.Sprintf("%s/resource-quota.yaml", appPath):         []byte(replaceVars(resourceQuotaTemplate, vars)),
		fmt.Sprintf("%s/network-policies.yaml", appPath):     []byte(replaceVars(networkPoliciesTemplate, vars)),
		fmt.Sprintf("%s/pull-secret-sync.yaml", appPath):       []byte(replaceVars(pullSecretSyncTemplate, vars)),
		fmt.Sprintf("%s/kustomization.yaml", appPath):          []byte(replaceVars(kustomizationTemplate, vars)),
		fmt.Sprintf("argocd/imageupdater-%s.yaml", req.Name): []byte(replaceVars(imageUpdaterTemplate, vars)),
	}

	return files
}

func renderEnvVars(env map[string]string) string {
	if len(env) == 0 {
		return ""
	}

	keys := make([]string, 0, len(env))
	for k := range env {
		keys = append(keys, k)
	}
	// stable order
	for i := 0; i < len(keys); i++ {
		for j := i + 1; j < len(keys); j++ {
			if keys[j] < keys[i] {
				keys[i], keys[j] = keys[j], keys[i]
			}
		}
	}

	var b strings.Builder
	b.WriteString("          env:\n")
	for _, k := range keys {
		fmt.Fprintf(&b, "            - name: %s\n              value: %q\n", k, env[k])
	}
	return b.String()
}

func replaceVars(template string, vars map[string]string) string {
	out := template
	for k, v := range vars {
		out = strings.ReplaceAll(out, k, v)
	}
	return out
}

const namespaceTemplate = `apiVersion: v1
kind: Namespace
metadata:
  name: {{APP_NAME}}
  labels:
    tinycloud.io/managed-by: platform
    app.kubernetes.io/name: {{APP_NAME}}
`

const deploymentTemplate = `apiVersion: apps/v1
kind: Deployment
metadata:
  name: {{APP_NAME}}
  labels:
    app.kubernetes.io/name: {{APP_NAME}}
    app.kubernetes.io/managed-by: tinycloud
    tinycloud.io/managed-by: platform
spec:
  replicas: {{REPLICAS}}
  selector:
    matchLabels:
      app.kubernetes.io/name: {{APP_NAME}}
  template:
    metadata:
      labels:
        app.kubernetes.io/name: {{APP_NAME}}
        tinycloud.io/managed-by: platform
    spec:
      imagePullSecrets:
        - name: ocir-creds
      containers:
        - name: {{APP_NAME}}
          image: {{IMAGE}}:{{TAG}}
          imagePullPolicy: Always
          ports:
            - containerPort: {{PORT}}
              name: http
          resources:
            requests:
              cpu: 50m
              memory: 64Mi
            limits:
              cpu: 250m
              memory: 256Mi
{{ENV_VARS}}
`

const serviceTemplate = `apiVersion: v1
kind: Service
metadata:
  name: {{APP_NAME}}
  labels:
    app.kubernetes.io/name: {{APP_NAME}}
    tinycloud.io/managed-by: platform
spec:
  selector:
    app.kubernetes.io/name: {{APP_NAME}}
  ports:
    - port: 80
      targetPort: http
      name: http
`

const resourceQuotaTemplate = `apiVersion: v1
kind: ResourceQuota
metadata:
  name: {{APP_NAME}}-quota
spec:
  hard:
    requests.cpu: "500m"
    requests.memory: "512Mi"
    limits.cpu: "1000m"
    limits.memory: "1Gi"
    pods: "5"
`

const networkPoliciesTemplate = `apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: default-deny
spec:
  podSelector: {}
  policyTypes:
    - Ingress
    - Egress
---
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: allow-platform-ingress
spec:
  podSelector: {}
  policyTypes:
    - Ingress
  ingress:
    - from:
        - namespaceSelector:
            matchLabels:
              kubernetes.io/metadata.name: tinycloud
      ports:
        - protocol: TCP
          port: {{PORT}}
---
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: allow-monitoring-ingress
spec:
  podSelector: {}
  policyTypes:
    - Ingress
  ingress:
    - from:
        - namespaceSelector:
            matchLabels:
              kubernetes.io/metadata.name: monitoring
      ports:
        - protocol: TCP
          port: {{PORT}}
---
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: allow-dns-egress
spec:
  podSelector: {}
  policyTypes:
    - Egress
  egress:
    - to:
        - namespaceSelector:
            matchLabels:
              kubernetes.io/metadata.name: kube-system
          podSelector:
            matchLabels:
              k8s-app: kube-dns
      ports:
        - protocol: UDP
          port: 53
        - protocol: TCP
          port: 53
`

const pullSecretSyncTemplate = `apiVersion: v1
kind: ServiceAccount
metadata:
  name: pull-secret-sync
  annotations:
    argocd.argoproj.io/hook: PreSync
    argocd.argoproj.io/hook-delete-policy: BeforeHookCreation
    argocd.argoproj.io/sync-wave: "-3"
---
apiVersion: rbac.authorization.k8s.io/v1
kind: Role
metadata:
  name: pull-secret-sync
  annotations:
    argocd.argoproj.io/hook: PreSync
    argocd.argoproj.io/hook-delete-policy: BeforeHookCreation
    argocd.argoproj.io/sync-wave: "-3"
rules:
  - apiGroups: [""]
    resources: ["secrets"]
    verbs: ["get", "create", "update", "patch"]
---
apiVersion: rbac.authorization.k8s.io/v1
kind: RoleBinding
metadata:
  name: pull-secret-sync
  annotations:
    argocd.argoproj.io/hook: PreSync
    argocd.argoproj.io/hook-delete-policy: BeforeHookCreation
    argocd.argoproj.io/sync-wave: "-3"
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: Role
  name: pull-secret-sync
subjects:
  - kind: ServiceAccount
    name: pull-secret-sync
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: {{APP_NAME}}-ocir-creds-reader
  annotations:
    argocd.argoproj.io/hook: PreSync
    argocd.argoproj.io/hook-delete-policy: BeforeHookCreation
    argocd.argoproj.io/sync-wave: "-3"
rules:
  - apiGroups: [""]
    resources: ["secrets"]
    resourceNames: ["ocir-creds"]
    verbs: ["get"]
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: {{APP_NAME}}-ocir-creds-reader
  annotations:
    argocd.argoproj.io/hook: PreSync
    argocd.argoproj.io/hook-delete-policy: BeforeHookCreation
    argocd.argoproj.io/sync-wave: "-3"
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: {{APP_NAME}}-ocir-creds-reader
subjects:
  - kind: ServiceAccount
    name: pull-secret-sync
    namespace: {{APP_NAME}}
---
apiVersion: batch/v1
kind: Job
metadata:
  name: sync-ocir-creds
  annotations:
    argocd.argoproj.io/hook: PreSync
    argocd.argoproj.io/hook-delete-policy: BeforeHookCreation,HookSucceeded
    argocd.argoproj.io/sync-wave: "-1"
spec:
  ttlSecondsAfterFinished: 300
  template:
    spec:
      serviceAccountName: pull-secret-sync
      restartPolicy: Never
      containers:
        - name: sync
          image: bitnami/kubectl:latest
          resources:
            requests:
              cpu: 50m
              memory: 64Mi
            limits:
              cpu: 200m
              memory: 128Mi
          command:
            - /bin/bash
            - -ec
            - |
              kubectl get secret ocir-creds -n argocd -o yaml | \
                sed "s/namespace: argocd/namespace: {{APP_NAME}}/" | \
                grep -vE '^\s*(resourceVersion|uid|creationTimestamp):' | \
                kubectl apply -f -
`

const kustomizationTemplate = `apiVersion: kustomize.config.k8s.io/v1beta1
kind: Kustomization

namespace: {{APP_NAME}}

resources:
  - namespace.yaml
  - pull-secret-sync.yaml
  - deployment.yaml
  - service.yaml
  - resource-quota.yaml
  - network-policies.yaml

images:
  - name: {{IMAGE}}
    newTag: {{TAG}}
`

const imageUpdaterTemplate = `apiVersion: argocd-image-updater.argoproj.io/v1alpha1
kind: ImageUpdater
metadata:
  name: {{APP_NAME}}
  namespace: argocd
spec:
  commonUpdateSettings:
    updateStrategy: semver
    allowTags: "regexp:^v?[0-9]+\\.[0-9]+\\.[0-9]+(-[a-zA-Z0-9.]+)?$"
    pullSecret: pullsecret:argocd/ocir-creds

  writeBackConfig:
    method: git
    gitConfig:
      branch: main

  applicationRefs:
    - namePattern: {{APP_NAME}}
      images:
        - alias: app
          imageName: {{IMAGE}}
`
