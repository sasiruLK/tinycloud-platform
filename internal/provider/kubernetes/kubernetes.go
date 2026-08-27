// Package kubernetes is the Infra Provider for a bare Kubernetes cluster: the
// one that needs no cloud account of any kind.
//
// It answers the `/v0` contract from the Kubernetes API and metrics-server
// alone. Instances are the cluster's nodes, CPU and memory come from
// metrics-server when it is installed, and the ingress address comes from the
// ingress controller's Service. Alarms and backups are not implemented —
// Kubernetes has no cluster-native equivalent of either, and saying so is
// exactly what Capability discovery is for.
//
// This Provider is also how the contract was tested. If a Snapshot could not
// be assembled from the Kubernetes API, the source interfaces it is built on
// would have been Oracle-shaped, and the contract would have needed changing
// before publication rather than after.
package kubernetes

import (
	"context"
	"fmt"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"

	"github.com/sasiruLK/tinycloud-platform/internal/infra"
	"github.com/sasiruLK/tinycloud-platform/internal/provider"
)

// Well-known node labels. Both the current and the deprecated beta spelling are
// read: a cluster provisioned years ago still carries only the beta ones.
const (
	labelInstanceType     = "node.kubernetes.io/instance-type"
	labelInstanceTypeBeta = "beta.kubernetes.io/instance-type"
	labelZone             = "topology.kubernetes.io/zone"
	labelZoneBeta         = "failure-domain.beta.kubernetes.io/zone"
)

// Node states. The dashboard treats RUNNING as up and anything else as down, so
// a Ready node reports the same word every other Substrate uses for healthy.
const (
	stateRunning  = "RUNNING"
	stateNotReady = "NOT_READY"
)

// DefaultIngressService is where an ingress controller installed by the
// project's own manifests lives. An Instance whose controller is elsewhere
// says so in configuration.
const DefaultIngressService = "ingress-nginx/ingress-nginx-controller"

const bytesPerGiB = 1024 * 1024 * 1024

// Options configures the Provider.
type Options struct {
	// IngressService is "namespace/name" of the Service fronting the cluster.
	// Empty means DefaultIngressService.
	IngressService string
}

// Provider serves the Infra Capabilities a Kubernetes cluster can answer.
type Provider struct {
	nodes   kubernetes.Interface
	metrics NodeMetricsSource

	ingressNamespace string
	ingressName      string
}

// New returns a Provider reading through clientset, with node utilisation read
// through metrics. A nil metrics source means metrics-server is not available
// at all, which is not a failure: the CPU and memory series come back empty and
// the dashboard shows the utilisation it cannot know as null.
func New(clientset kubernetes.Interface, metrics NodeMetricsSource, opts Options) (*Provider, error) {
	service := opts.IngressService
	if service == "" {
		service = DefaultIngressService
	}
	namespace, name, ok := strings.Cut(service, "/")
	if !ok || namespace == "" || name == "" {
		return nil, fmt.Errorf("ingress service %q is not in namespace/name form", service)
	}

	return &Provider{
		nodes:            clientset,
		metrics:          metrics,
		ingressNamespace: namespace,
		ingressName:      name,
	}, nil
}

// Name identifies this Provider in Core's warnings.
func (p *Provider) Name() string { return "kubernetes" }

// Capabilities are the three a cluster can answer honestly. Alarms and backups
// are absent rather than empty: an empty alarm list reads as "nothing is
// wrong", which is a claim this Provider is in no position to make.
func (p *Provider) Capabilities() []string {
	return []string{
		provider.CapabilityInstances,
		provider.CapabilityMetrics,
		provider.CapabilityIngress,
	}
}

// Instances maps the cluster's nodes onto the contract's machines.
func (p *Provider) Instances(ctx context.Context) ([]infra.InstanceInfo, error) {
	list, err := p.nodes.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, provider.Upstream(fmt.Errorf("list nodes: %w", err))
	}

	out := make([]infra.InstanceInfo, 0, len(list.Items))
	for i := range list.Items {
		out = append(out, instanceFromNode(&list.Items[i]))
	}
	return out, nil
}

func instanceFromNode(node *corev1.Node) infra.InstanceInfo {
	info := infra.InstanceInfo{
		ID:          string(node.UID),
		Name:        node.Name,
		State:       stateNotReady,
		Shape:       firstLabel(node, labelInstanceType, labelInstanceTypeBeta),
		FaultDomain: firstLabel(node, labelZone, labelZoneBeta),
	}

	for _, cond := range node.Status.Conditions {
		if cond.Type == corev1.NodeReady && cond.Status == corev1.ConditionTrue {
			info.State = stateRunning
			break
		}
	}

	// A cluster off a cloud has no instance-type label at all. Its architecture
	// is the truest thing left to say about the machine's shape, and is more
	// use on the dashboard than an empty cell.
	if info.Shape == "" {
		info.Shape = node.Status.NodeInfo.Architecture
	}

	if cpu := node.Status.Capacity.Cpu(); cpu != nil {
		info.OCPUs = cpu.AsApproximateFloat64()
	}
	if mem := node.Status.Capacity.Memory(); mem != nil {
		info.MemoryGB = mem.AsApproximateFloat64() / bytesPerGiB
	}

	for _, addr := range node.Status.Addresses {
		if addr.Type == corev1.NodeInternalIP {
			info.PrivateIP = addr.Address
			break
		}
	}
	return info
}

func firstLabel(node *corev1.Node, keys ...string) string {
	for _, key := range keys {
		if v := node.Labels[key]; v != "" {
			return v
		}
	}
	return ""
}

// Metric answers the utilisation metrics from metrics-server, keyed by node
// name so that Core can match them to the instances it was given.
//
// The metrics a cluster has no equivalent of — load balancer backend counts,
// synthetic uptime — return no series rather than an error. The contract says
// absent is not broken, and the dashboard renders those as null.
func (p *Provider) Metric(ctx context.Context, metric string, _ time.Duration) ([]infra.Series, error) {
	switch metric {
	case infra.MetricCPUUtilization, infra.MetricMemoryUtilization:
	default:
		return nil, nil
	}

	if p.metrics == nil {
		return nil, nil
	}

	usage, err := p.metrics.NodeMetrics(ctx)
	if err != nil {
		if IsMetricsUnavailable(err) {
			// metrics-server is not installed. That is a normal state for a
			// fresh cluster, so the Capability degrades to absent values rather
			// than reporting the Substrate as broken.
			return nil, nil
		}
		return nil, provider.Upstream(fmt.Errorf("read node metrics: %w", err))
	}

	nodes, err := p.nodes.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, provider.Upstream(fmt.Errorf("list nodes: %w", err))
	}

	// Utilisation is usage against the node's own capacity: metrics-server
	// reports cores and bytes, and the contract wants a percentage.
	capacity := make(map[string]corev1.ResourceList, len(nodes.Items))
	for i := range nodes.Items {
		capacity[nodes.Items[i].Name] = nodes.Items[i].Status.Capacity
	}

	series := make([]infra.Series, 0, len(usage))
	for _, u := range usage {
		limits, known := capacity[u.Name]
		if !known {
			continue
		}

		var used, total float64
		switch metric {
		case infra.MetricCPUUtilization:
			used, total = u.CPUCores, limits.Cpu().AsApproximateFloat64()
		case infra.MetricMemoryUtilization:
			used, total = u.MemoryBytes, limits.Memory().AsApproximateFloat64()
		}
		if total <= 0 {
			continue
		}

		series = append(series, infra.Series{
			Dimensions: map[string]string{infra.DimInstance: u.Name},
			Timestamp:  u.Timestamp.UTC(),
			Value:      used / total * 100,
		})
	}
	return series, nil
}

// Alarms is not implemented: Kubernetes has no cluster-native alarm concept,
// and an empty list would read as "nothing is wrong".
func (p *Provider) Alarms(context.Context) ([]infra.AlarmStatus, error) {
	return nil, provider.ErrNotImplemented
}

// BackupObjects is not implemented: a cluster has no backup store of its own.
func (p *Provider) BackupObjects(context.Context) ([]infra.ObjectInfo, error) {
	return nil, provider.ErrNotImplemented
}

// IngressAddress returns the external address of the ingress controller's
// Service. An address that has not been assigned yet is the empty string: on a
// cluster with no load balancer integration, pending is where it stays, and
// that is a state rather than a failure.
func (p *Provider) IngressAddress(ctx context.Context) (string, error) {
	service, err := p.nodes.CoreV1().Services(p.ingressNamespace).Get(ctx, p.ingressName, metav1.GetOptions{})
	if err != nil {
		return "", provider.Upstream(fmt.Errorf("get service %s/%s: %w", p.ingressNamespace, p.ingressName, err))
	}

	for _, ingress := range service.Status.LoadBalancer.Ingress {
		if ingress.IP != "" {
			return ingress.IP, nil
		}
		if ingress.Hostname != "" {
			return ingress.Hostname, nil
		}
	}
	// A NodePort or ClusterIP controller has no external address to report; an
	// operator fronting it themselves knows the address the Provider cannot.
	return "", nil
}
