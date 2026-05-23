package k8s

import (
	"context"
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

// GetPodLogs returns logs from a pod
func (c *Client) GetPodLogs(ctx context.Context, namespace, podName, container string, tailLines int64) (string, error) {
	req := c.K8s.CoreV1().Pods(namespace).GetLogs(podName, &corev1.PodLogOptions{
		Container:  container,
		TailLines:  &tailLines,
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
