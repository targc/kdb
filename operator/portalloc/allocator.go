package portalloc

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	configMapName      = "kdb-port-allocations"
	defaultPortRange   = "6100-6199"
	lbNodeLabel        = "kdb/role"
	lbNodeValue        = "lb"
	portRangeAnnotation = "kdb.io/port-range"
	hostAnnotation      = "kdb.io/host"
)

// Allocation holds the result of a port allocation.
type Allocation struct {
	Node string // LB node name
	Host string // LB node IP (InternalIP)
	Port int32
}

type Allocator struct {
	client    client.Client
	namespace string
	minPort   int32
	maxPort   int32
}

func New(c client.Client, namespace string) *Allocator {
	min, max := parseRange(os.Getenv("KDB_PORT_RANGE"))
	return &Allocator{
		client:    c,
		namespace: namespace,
		minPort:   min,
		maxPort:   max,
	}
}

// Allocate finds an LB node with a free port and assigns it to the resource.
// If already allocated, returns the existing allocation.
// Retries on conflict (optimistic concurrency).
func (a *Allocator) Allocate(ctx context.Context, resourceKey string) (*Allocation, error) {
	const maxRetries = 5
	for attempt := 0; attempt < maxRetries; attempt++ {
		alloc, err := a.tryAllocate(ctx, resourceKey)
		if err == nil {
			return alloc, nil
		}
		if !errors.IsConflict(err) {
			return nil, fmt.Errorf("failed to allocate port: %w", err)
		}
	}
	return nil, fmt.Errorf("failed to allocate port: too many conflicts")
}

func (a *Allocator) tryAllocate(ctx context.Context, resourceKey string) (*Allocation, error) {
	cm, err := a.getOrCreateConfigMap(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get port allocations: %w", err)
	}

	// Check if already allocated (format: "node_port" → resourceKey)
	for key, val := range cm.Data {
		if val == resourceKey {
			node, port := parseKey(key)
			host, _ := a.getNodeHost(ctx, node)
			return &Allocation{Node: node, Host: host, Port: port}, nil
		}
	}

	// Find LB nodes
	lbNodes, err := a.getLBNodes(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list LB nodes: %w", err)
	}
	if len(lbNodes) == 0 {
		return nil, fmt.Errorf("no LB nodes found (label %s=%s)", lbNodeLabel, lbNodeValue)
	}

	// Build used ports per node
	usedPerNode := make(map[string]map[int32]bool)
	for _, n := range lbNodes {
		usedPerNode[n.Name] = make(map[int32]bool)
	}
	for key := range cm.Data {
		node, port := parseKey(key)
		if _, ok := usedPerNode[node]; ok {
			usedPerNode[node][port] = true
		}
	}

	// Find first node with a free port (respecting per-node port range)
	for _, n := range lbNodes {
		used := usedPerNode[n.Name]
		minP, maxP := a.nodePortRange(n)
		for port := minP; port <= maxP; port++ {
			if !used[port] {
				allocKey := fmt.Sprintf("%s_%d", n.Name, port)
				if cm.Data == nil {
					cm.Data = make(map[string]string)
				}
				cm.Data[allocKey] = resourceKey
				if err := a.client.Update(ctx, cm); err != nil {
					return nil, err
				}
				host := nodeHost(n)
				return &Allocation{Node: n.Name, Host: host, Port: port}, nil
			}
		}
	}

	return nil, fmt.Errorf("no free ports on any LB node (range %d-%d)", a.minPort, a.maxPort)
}

// Release frees the allocation for the given resource.
func (a *Allocator) Release(ctx context.Context, resourceKey string) error {
	cm, err := a.getOrCreateConfigMap(ctx)
	if err != nil {
		return fmt.Errorf("failed to get port allocations: %w", err)
	}

	for key, val := range cm.Data {
		if val == resourceKey {
			delete(cm.Data, key)
			if err := a.client.Update(ctx, cm); err != nil {
				return fmt.Errorf("failed to release port: %w", err)
			}
			return nil
		}
	}
	return nil
}

// nodePortRange returns the port range for a node.
// Uses annotation kdb.io/port-range if set, otherwise falls back to global range.
func (a *Allocator) nodePortRange(node corev1.Node) (int32, int32) {
	if ann, ok := node.Annotations[portRangeAnnotation]; ok {
		min, max := parseRange(ann)
		if min > 0 && max > 0 {
			return min, max
		}
	}
	return a.minPort, a.maxPort
}

func (a *Allocator) getLBNodes(ctx context.Context) ([]corev1.Node, error) {
	nodeList := &corev1.NodeList{}
	if err := a.client.List(ctx, nodeList, client.MatchingLabels{lbNodeLabel: lbNodeValue}); err != nil {
		return nil, err
	}
	return nodeList.Items, nil
}

func (a *Allocator) getNodeHost(ctx context.Context, nodeName string) (string, error) {
	node := &corev1.Node{}
	if err := a.client.Get(ctx, client.ObjectKey{Name: nodeName}, node); err != nil {
		return "", err
	}
	return nodeHost(*node), nil
}

func nodeHost(node corev1.Node) string {
	if h, ok := node.Annotations[hostAnnotation]; ok && h != "" {
		return h
	}
	for _, addr := range node.Status.Addresses {
		if addr.Type == corev1.NodeInternalIP {
			return addr.Address
		}
	}
	return node.Name
}

func (a *Allocator) getOrCreateConfigMap(ctx context.Context) (*corev1.ConfigMap, error) {
	cm := &corev1.ConfigMap{}
	key := client.ObjectKey{Name: configMapName, Namespace: a.namespace}

	if err := a.client.Get(ctx, key, cm); err != nil {
		if !errors.IsNotFound(err) {
			return nil, err
		}
		cm = &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      configMapName,
				Namespace: a.namespace,
			},
			Data: make(map[string]string),
		}
		if err := a.client.Create(ctx, cm); err != nil {
			if errors.IsAlreadyExists(err) {
				return cm, a.client.Get(ctx, key, cm)
			}
			return nil, err
		}
	}
	return cm, nil
}

// parseKey splits "node-name_6100" into node and port.
func parseKey(key string) (string, int32) {
	i := strings.LastIndex(key, "_")
	if i < 0 {
		return "", 0
	}
	port, _ := strconv.ParseInt(key[i+1:], 10, 32)
	return key[:i], int32(port)
}

func parseRange(s string) (int32, int32) {
	if s == "" {
		s = defaultPortRange
	}
	parts := strings.SplitN(s, "-", 2)
	if len(parts) != 2 {
		parts = strings.SplitN(defaultPortRange, "-", 2)
	}
	min, _ := strconv.ParseInt(parts[0], 10, 32)
	max, _ := strconv.ParseInt(parts[1], 10, 32)
	return int32(min), int32(max)
}
