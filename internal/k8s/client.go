package k8s

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

// Client wraps Kubernetes and dynamic clients
type Client struct {
	K8s       *kubernetes.Clientset
	Dynamic   dynamic.Interface
	ArgoCDGVR schema.GroupVersionResource
}

// NewClient creates a Kubernetes client (in-cluster or from kubeconfig)
func NewClient() (*Client, error) {
	var config *rest.Config
	var err error

	kubeconfig := os.Getenv("KUBECONFIG")
	if kubeconfig != "" {
		config, err = clientcmd.BuildConfigFromFlags("", kubeconfig)
	} else {
		config, err = rest.InClusterConfig()
	}
	if err != nil {
		return nil, fmt.Errorf("failed to build k8s config: %w", err)
	}

	k8sClient, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create k8s client: %w", err)
	}

	dynamicClient, err := dynamic.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create dynamic client: %w", err)
	}

	argoGVR := schema.GroupVersionResource{
		Group:    "argoproj.io",
		Version:  "v1alpha1",
		Resource: "applications",
	}

	return &Client{
		K8s:       k8sClient,
		Dynamic:   dynamicClient,
		ArgoCDGVR: argoGVR,
	}, nil
}

// ListApplications returns all Argo CD Applications in the argocd namespace
func (c *Client) ListApplications(ctx context.Context) (*unstructured.UnstructuredList, error) {
	return c.Dynamic.Resource(c.ArgoCDGVR).Namespace("argocd").List(ctx, metav1.ListOptions{})
}

// GetApplication returns a single Argo CD Application
func (c *Client) GetApplication(ctx context.Context, name string) (*unstructured.Unstructured, error) {
	return c.Dynamic.Resource(c.ArgoCDGVR).Namespace("argocd").Get(ctx, name, metav1.GetOptions{})
}

// TriggerSync patches an Argo CD Application to trigger a sync
func (c *Client) TriggerSync(ctx context.Context, name string) error {
	patch := []byte(`{"operation":{"sync":{"revision":"HEAD","prune":true}}}`)
	_, err := c.Dynamic.Resource(c.ArgoCDGVR).Namespace("argocd").Patch(
		ctx, name, types.MergePatchType, patch, metav1.PatchOptions{},
	)
	return err
}

// PatchTargetRevision patches the Argo CD Application's targetRevision
func (c *Client) PatchTargetRevision(ctx context.Context, name, targetRevision string) error {
	patch := []byte(fmt.Sprintf(`{"spec":{"source":{"targetRevision":"%s"}}}`, targetRevision))
	_, err := c.Dynamic.Resource(c.ArgoCDGVR).Namespace("argocd").Patch(
		ctx, name, types.MergePatchType, patch, metav1.PatchOptions{},
	)
	return err
}

// GetAppTargetRevision returns the current targetRevision of an Argo CD Application
func (c *Client) GetAppTargetRevision(ctx context.Context, name string) (string, error) {
	app, err := c.GetApplication(ctx, name)
	if err != nil {
		return "", err
	}
	rev, _, _ := unstructured.NestedString(app.Object, "spec", "source", "targetRevision")
	return rev, nil
}

// GetPodLogs returns logs from a pod
func (c *Client) GetPodLogs(ctx context.Context, namespace, podName, container string, tailLines int64) (string, error) {
	req := c.K8s.CoreV1().Pods(namespace).GetLogs(podName, &corev1.PodLogOptions{
		Container: container,
		TailLines: &tailLines,
	})
	stream, err := req.Stream(ctx)
	if err != nil {
		return "", err
	}
	defer stream.Close()

	buf := new(strings.Builder)
	_, err = io.Copy(buf, stream)
	if err != nil {
		return "", err
	}
	return buf.String(), nil
}

// GetDeploymentPods returns pods owned by a Deployment
func (c *Client) GetDeploymentPods(ctx context.Context, namespace, deploymentName string) (*corev1.PodList, error) {
	// Find pods with label selector matching deployment
	deploy, err := c.K8s.AppsV1().Deployments(namespace).Get(ctx, deploymentName, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}

	selector := metav1.FormatLabelSelector(deploy.Spec.Selector)
	return c.K8s.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{
		LabelSelector: selector,
	})
}

// ResourceNode is one node of an application's resource tree.
//
// Argo CD's Application only records a flat list in .status.resources, with no
// parent/child information, so a Deployment and its Pods appear as unrelated
// entries. This reconstructs the hierarchy from Kubernetes ownerReferences,
// which is the same relationship Argo's own UI draws.
type ResourceNode struct {
	Kind      string         `json:"kind"`
	Name      string         `json:"name"`
	Namespace string         `json:"namespace,omitempty"`
	Status    string         `json:"status,omitempty"`
	Health    string         `json:"health,omitempty"`
	Detail    string         `json:"detail,omitempty"`
	Children  []ResourceNode `json:"children,omitempty"`
}

// BuildResourceTree expands workloads in a namespace into their ReplicaSets and
// Pods. Anything with no children — Services, ConfigMaps — comes back as a leaf.
//
// Errors listing children are deliberately swallowed: a partial tree is far more
// useful than an error page, and the top-level resources are already known good
// from the Application status.
func (c *Client) BuildResourceTree(ctx context.Context, namespace string, tops []ResourceNode) []ResourceNode {
	if namespace == "" {
		return tops
	}

	rsList, err := c.K8s.AppsV1().ReplicaSets(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return tops
	}
	podList, err := c.K8s.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return tops
	}

	// Pods indexed by the UID of whatever owns them.
	podsByOwner := map[string][]ResourceNode{}
	for _, p := range podList.Items {
		node := ResourceNode{
			Kind:      "Pod",
			Name:      p.Name,
			Namespace: p.Namespace,
			Status:    string(p.Status.Phase),
			Health:    podHealth(&p),
			Detail:    podDetail(&p),
		}
		for _, o := range p.OwnerReferences {
			podsByOwner[string(o.UID)] = append(podsByOwner[string(o.UID)], node)
		}
	}

	// ReplicaSets indexed by owning workload name, carrying their pods. Scaled-down
	// historical ReplicaSets are skipped — Argo shows them, but for this platform
	// they are noise that hides the live one.
	rsByOwner := map[string][]ResourceNode{}
	for _, rs := range rsList.Items {
		kids := podsByOwner[string(rs.UID)]
		if len(kids) == 0 && rs.Status.Replicas == 0 {
			continue
		}
		node := ResourceNode{
			Kind:      "ReplicaSet",
			Name:      rs.Name,
			Namespace: rs.Namespace,
			Status:    fmt.Sprintf("%d/%d", rs.Status.ReadyReplicas, rs.Status.Replicas),
			Children:  kids,
		}
		for _, o := range rs.OwnerReferences {
			rsByOwner[o.Name] = append(rsByOwner[o.Name], node)
		}
	}

	out := make([]ResourceNode, 0, len(tops))
	for _, t := range tops {
		if t.Kind == "Deployment" {
			t.Children = rsByOwner[t.Name]
		}
		if t.Namespace == "" {
			t.Namespace = namespace
		}
		out = append(out, t)
	}
	return out
}

func podHealth(p *corev1.Pod) string {
	for _, cs := range p.Status.ContainerStatuses {
		if cs.State.Waiting != nil && cs.State.Waiting.Reason != "" {
			return cs.State.Waiting.Reason
		}
		if !cs.Ready && p.Status.Phase == corev1.PodRunning {
			return "NotReady"
		}
	}
	if p.Status.Phase == corev1.PodRunning {
		return "Healthy"
	}
	return string(p.Status.Phase)
}

// podDetail surfaces the thing you actually want when a pod is unhappy: how many
// times it has restarted, and which node it is on.
func podDetail(p *corev1.Pod) string {
	var restarts int32
	for _, cs := range p.Status.ContainerStatuses {
		restarts += cs.RestartCount
	}
	if restarts > 0 {
		return fmt.Sprintf("%d restarts · %s", restarts, p.Spec.NodeName)
	}
	return p.Spec.NodeName
}

// GetResourceManifest returns the live object for one node of the resource
// graph, so clicking a node can show what is actually running rather than what
// git says should be.
//
// Typed clients for the four kinds the graph produces, rather than a RESTMapper
// over the dynamic client: this platform generates a known, small set of kinds,
// and an explicit switch fails loudly on anything unexpected instead of
// silently returning nothing.
func (c *Client) GetResourceManifest(ctx context.Context, namespace, kind, name string) (map[string]any, error) {
	if namespace == "" || name == "" {
		return nil, fmt.Errorf("namespace and name are required")
	}

	var obj any
	var err error

	switch kind {
	case "Pod":
		obj, err = c.K8s.CoreV1().Pods(namespace).Get(ctx, name, metav1.GetOptions{})
	case "Service":
		obj, err = c.K8s.CoreV1().Services(namespace).Get(ctx, name, metav1.GetOptions{})
	case "Deployment":
		obj, err = c.K8s.AppsV1().Deployments(namespace).Get(ctx, name, metav1.GetOptions{})
	case "ReplicaSet":
		obj, err = c.K8s.AppsV1().ReplicaSets(namespace).Get(ctx, name, metav1.GetOptions{})
	default:
		return nil, fmt.Errorf("unsupported kind %q", kind)
	}
	if err != nil {
		return nil, err
	}

	// Round-trip through JSON so the response is the plain object, and strip the
	// noise Kubernetes adds that obscures the parts worth reading.
	raw, err := json.Marshal(obj)
	if err != nil {
		return nil, err
	}
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, err
	}
	if md, ok := out["metadata"].(map[string]any); ok {
		delete(md, "managedFields")
		if ann, ok := md["annotations"].(map[string]any); ok {
			delete(ann, "kubectl.kubernetes.io/last-applied-configuration")
		}
	}
	out["kind"] = kind
	return out, nil
}

// GetPodContainers lists container names, so a multi-container pod can be asked
// for the right one rather than defaulting to the first and looking empty.
func (c *Client) GetPodContainers(ctx context.Context, namespace, podName string) ([]string, error) {
	pod, err := c.K8s.CoreV1().Pods(namespace).Get(ctx, podName, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(pod.Spec.Containers))
	for _, ct := range pod.Spec.Containers {
		names = append(names, ct.Name)
	}
	return names, nil
}
