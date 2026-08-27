package kubernetes

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/client-go/rest"
)

// NodeMetric is one node's current resource usage, as metrics-server reports it.
type NodeMetric struct {
	Name        string
	CPUCores    float64
	MemoryBytes float64
	Timestamp   time.Time
}

// NodeMetricsSource reads current node usage. It is an interface so the
// Provider can be tested without a metrics API, and so that a cluster without
// metrics-server can be represented by the absence of one.
type NodeMetricsSource interface {
	NodeMetrics(ctx context.Context) ([]NodeMetric, error)
}

// ErrMetricsUnavailable reports that the metrics API is not served by this
// cluster — metrics-server is not installed. It is not a failure: the Provider
// answers the metric Capability with no series, and utilisation reaches the
// dashboard as null.
var ErrMetricsUnavailable = errors.New("metrics API not available on this cluster")

// IsMetricsUnavailable reports whether err means metrics-server is absent
// rather than broken.
func IsMetricsUnavailable(err error) bool { return errors.Is(err, ErrMetricsUnavailable) }

// metricsAPIPath is the metrics-server node endpoint. It is read through the
// generic REST client rather than through a generated client so that this
// Provider adds no dependency beyond the Kubernetes libraries the project
// already has.
const metricsAPIPath = "/apis/metrics.k8s.io/v1beta1/nodes"

// restNodeMetrics reads metrics-server through a cluster REST client.
type restNodeMetrics struct{ client rest.Interface }

// NewRESTNodeMetrics returns a metrics source reading the cluster's metrics API.
func NewRESTNodeMetrics(client rest.Interface) NodeMetricsSource {
	return &restNodeMetrics{client: client}
}

// nodeMetricsList is the shape metrics-server answers with. Only the fields
// this Provider uses are decoded.
type nodeMetricsList struct {
	Items []struct {
		Metadata struct {
			Name string `json:"name"`
		} `json:"metadata"`
		Timestamp time.Time `json:"timestamp"`
		Usage     struct {
			CPU    string `json:"cpu"`
			Memory string `json:"memory"`
		} `json:"usage"`
	} `json:"items"`
}

func (r *restNodeMetrics) NodeMetrics(ctx context.Context) ([]NodeMetric, error) {
	raw, err := r.client.Get().AbsPath(metricsAPIPath).DoRaw(ctx)
	if err != nil {
		if apierrors.IsNotFound(err) || apierrors.IsServiceUnavailable(err) {
			// The API group is not registered, or its backing service has no
			// endpoints: either way this cluster has no metrics-server.
			return nil, fmt.Errorf("%s: %w", err, ErrMetricsUnavailable)
		}
		return nil, fmt.Errorf("query %s: %w", metricsAPIPath, err)
	}

	var list nodeMetricsList
	if err := json.Unmarshal(raw, &list); err != nil {
		return nil, fmt.Errorf("decode node metrics: %w", err)
	}

	out := make([]NodeMetric, 0, len(list.Items))
	for _, item := range list.Items {
		cpu, err := resource.ParseQuantity(item.Usage.CPU)
		if err != nil {
			return nil, fmt.Errorf("node %s: parse cpu usage %q: %w", item.Metadata.Name, item.Usage.CPU, err)
		}
		memory, err := resource.ParseQuantity(item.Usage.Memory)
		if err != nil {
			return nil, fmt.Errorf("node %s: parse memory usage %q: %w", item.Metadata.Name, item.Usage.Memory, err)
		}

		out = append(out, NodeMetric{
			Name:        item.Metadata.Name,
			CPUCores:    cpu.AsApproximateFloat64(),
			MemoryBytes: memory.AsApproximateFloat64(),
			Timestamp:   item.Timestamp,
		})
	}
	return out, nil
}
