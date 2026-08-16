package manifests

import (
	"fmt"
	"regexp"
	"strings"
)

const (
	// Matches the pod count in resourceQuotaTemplate; a 2-node/12GB lab cannot
	// sensibly run more, and the quota would reject anything higher anyway.
	maxReplicas = 5
	minReplicas = 0
	appPort     = 8080
)

var (
	appNameRegex = regexp.MustCompile(`^[a-z0-9]([-a-z0-9]*[a-z0-9])?$`)
	// Tags are immutable 40-character git commit SHAs. This must stay in sync
	// with the allowTags regexp in imageUpdaterTemplate below: a tag the API
	// accepts but Image Updater will not match means the app can never receive
	// an automated update.
	commitSHARegex = regexp.MustCompile(`^[a-f0-9]{40}$`)
	imageRegex     = regexp.MustCompile(`^[a-z0-9]+(?:[._-][a-z0-9]+)*(?:/[a-z0-9]+(?:[._-][a-z0-9]+)*)+$`)
	reservedNames  = map[string]bool{
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

// PlaceholderTag satisfies tag validation when the real tag is not yet known.
// Build requests are validated up front, but the commit SHA that becomes the
// image tag is only resolved later, by the executor, once it has cloned the
// repository and read HEAD. Callers validating a not-yet-built app must use
// this rather than inventing a literal, which is how a semver dummy survived
// the switch to commit-SHA tags and broke every build request.
const PlaceholderTag = "0000000000000000000000000000000000000000"

// PlatformHost is the external platform hostname.
const PlatformHost = "tinycloud.sasiru.lk"

// AppDomain is the external application wildcard domain.
const AppDomain = "sasiru.lk"

// PlatformBaseURL is the external platform URL for platform links.
const PlatformBaseURL = "https://" + PlatformHost

// AppBaseURL returns the public application URL.
func AppBaseURL(name string) string {
	return fmt.Sprintf("https://%s.%s/", strings.TrimSpace(name), AppDomain)
}

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
	if !commitSHARegex.MatchString(tag) {
		return fmt.Errorf("tag must be a 40-character git commit SHA (lowercase hex)")
	}

	if req.Replicas < 1 || req.Replicas > maxReplicas {
		return fmt.Errorf("replicas must be between 1 and %d", maxReplicas)
	}

	if req.Port != appPort {
		return fmt.Errorf("port must be %d for the current platform contract", appPort)
	}
	if envPort, ok := req.Env["PORT"]; ok && strings.TrimSpace(envPort) != fmt.Sprintf("%d", appPort) {
		return fmt.Errorf("env PORT must be %d for the current platform contract", appPort)
	}

	return nil
}

// NormalizeCreateAppRequest trims and applies defaults
func NormalizeCreateAppRequest(req *CreateAppRequest) {
	req.Name = strings.TrimSpace(req.Name)
	req.Image = strings.TrimSpace(req.Image)
	req.Tag = strings.TrimSpace(req.Tag)
	req.Port = appPort
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
		fmt.Sprintf("%s/namespace.yaml", appPath):            []byte(replaceVars(namespaceTemplate, vars)),
		fmt.Sprintf("%s/deployment.yaml", appPath):           []byte(replaceVars(deploymentTemplate, vars)),
		fmt.Sprintf("%s/service.yaml", appPath):              []byte(replaceVars(serviceTemplate, vars)),
		fmt.Sprintf("%s/resource-quota.yaml", appPath):       []byte(replaceVars(resourceQuotaTemplate, vars)),
		fmt.Sprintf("%s/network-policies.yaml", appPath):     []byte(replaceVars(networkPoliciesTemplate, vars)),
		fmt.Sprintf("%s/pull-secret.yaml", appPath):          []byte(replaceVars(pullSecretTemplate, vars)),
		fmt.Sprintf("%s/kustomization.yaml", appPath):        []byte(replaceVars(kustomizationTemplate, vars)),
		fmt.Sprintf("argocd/imageupdater-%s.yaml", req.Name): []byte(replaceVars(imageUpdaterTemplate, vars)),
	}

	return files
}

func renderEnvVars(env map[string]string) string {
	keys := make([]string, 0, len(env))
	for k := range env {
		if k == "PORT" {
			continue
		}
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
	fmt.Fprintf(&b, "            - name: PORT\n              value: %q\n", fmt.Sprintf("%d", appPort))
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
        - name: ghcr-creds
      containers:
        - name: {{APP_NAME}}
          image: {{IMAGE}}:{{TAG}}
          imagePullPolicy: Always
          ports:
            - containerPort: {{PORT}}
              name: http
{{ENV_VARS}}
          # Probes check for a listening socket rather than an HTTP health path,
          # so an app needs no particular endpoint to deploy healthily.
          readinessProbe:
            tcpSocket:
              port: http
            initialDelaySeconds: 5
            periodSeconds: 5
          livenessProbe:
            tcpSocket:
              port: http
            initialDelaySeconds: 15
            periodSeconds: 10
          # Requests sized near real usage, not far below it. The scheduler packs
          # by requests alone, so a 4x gap between request and limit let it keep
          # placing pods on a node that reported 11% memory while actually being
          # close to full.
          resources:
            requests:
              cpu: 100m
              memory: 128Mi
            limits:
              cpu: 250m
              memory: 256Mi
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
  # Sized to hold maxReplicas pods at the deployment's requests and limits.
  # Previously an app could pass validation at 10 replicas and then be refused
  # by this quota at 6.
  hard:
    requests.cpu: "600m"
    requests.memory: "704Mi"
    limits.cpu: "1500m"
    limits.memory: "1408Mi"
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

// The image pull secret, read from OCI Vault into the app's namespace.
//
// This replaced a PreSync hook that ran a Job to copy ghcr-creds out of another
// namespace. That needed seven objects per app -- a ServiceAccount, a Role, a
// RoleBinding, a NetworkPolicy opening apiserver egress, the Job itself and its
// hook annotations -- and gave every app namespace a credential-copying
// identity that could read Secrets elsewhere in the cluster.
//
// One ExternalSecret does the same job with no Job, no RBAC and no apiserver
// egress, and it re-reconciles on its own if the credential is rotated in Vault
// instead of staying stale until the next sync.
//
// creationPolicy is Orphan so that removing this file, or the app's
// Application, does not garbage-collect the pull secret out from under running
// pods that still need it to restart.
const pullSecretTemplate = `apiVersion: external-secrets.io/v1beta1
kind: ExternalSecret
metadata:
  name: ghcr-creds
  namespace: {{APP_NAME}}
  labels:
    app.kubernetes.io/name: {{APP_NAME}}
    app.kubernetes.io/managed-by: tinycloud
spec:
  refreshInterval: 1h
  secretStoreRef:
    name: oci-vault
    kind: ClusterSecretStore
  target:
    name: ghcr-creds
    creationPolicy: Orphan
    deletionPolicy: Retain
    template:
      # CRD defaults, written out so the generated app does not sit permanently
      # OutOfSync against fields the API server fills in.
      engineVersion: v2
      mergePolicy: Replace
      type: kubernetes.io/dockerconfigjson
      data:
        .dockerconfigjson: '{{ index . ".dockerconfigjson" }}'
  dataFrom:
    - extract:
        key: ghcr-creds
        # CRD defaults again, on the extract this time. The template block above
        # needed the same treatment; both are fields the API server fills in and
        # git has to claim, or the diff never closes.
        conversionStrategy: Default
        decodingStrategy: None
        metadataPolicy: None
`

const kustomizationTemplate = `apiVersion: kustomize.config.k8s.io/v1beta1
kind: Kustomization

namespace: {{APP_NAME}}

resources:
  - namespace.yaml
  - pull-secret.yaml
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
    updateStrategy: newest-build
    allowTags: "regexp:^[a-f0-9]{40}$"
    pullSecret: pullsecret:argocd/ghcr-creds

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
