package metrics

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"kdb.io/operator/mongo"
	"kdb.io/operator/postgres"
	"kdb.io/operator/redis"
)

type ResourceMetrics struct {
	UID          string         `json:"uid"`
	APIVersion   string         `json:"api_version"`
	Kind         string         `json:"kind"`
	Name         string         `json:"name"`
	Namespace    string         `json:"namespace"`
	Metrics      map[string]any `json:"metrics"`
}

type Handler struct {
	client client.Client
}

func NewHandler(c client.Client) *Handler {
	return &Handler{client: c}
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	now := time.Now()

	var results []ResourceMetrics

	h.collectPostgres(ctx, now, &results)
	h.collectMongo(ctx, now, &results)
	h.collectRedis(ctx, now, &results)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(results)
}

func (h *Handler) collectPostgres(ctx context.Context, now time.Time, results *[]ResourceMetrics) {
	var list postgres.PostgresList
	if err := h.client.List(ctx, &list); err != nil {
		return
	}
	for _, item := range list.Items {
		m := h.buildMetrics(ctx, now, item.Name, item.Namespace, item.Spec.Storage.Size)
		*results = append(*results, ResourceMetrics{
			UID:          string(item.UID),
			APIVersion:   postgres.GroupVersion.String(),
			Kind:         "Postgres",
			Name:         item.Name,
			Namespace:    item.Namespace,
			Metrics:      m,
		})
	}
}

func (h *Handler) collectMongo(ctx context.Context, now time.Time, results *[]ResourceMetrics) {
	var list mongo.MongoList
	if err := h.client.List(ctx, &list); err != nil {
		return
	}
	for _, item := range list.Items {
		m := h.buildMetrics(ctx, now, item.Name, item.Namespace, item.Spec.Storage.Size)
		*results = append(*results, ResourceMetrics{
			UID:          string(item.UID),
			APIVersion:   mongo.GroupVersion.String(),
			Kind:         "Mongo",
			Name:         item.Name,
			Namespace:    item.Namespace,
			Metrics:      m,
		})
	}
}

func (h *Handler) collectRedis(ctx context.Context, now time.Time, results *[]ResourceMetrics) {
	var list redis.RedisList
	if err := h.client.List(ctx, &list); err != nil {
		return
	}
	for _, item := range list.Items {
		m := h.buildMetrics(ctx, now, item.Name, item.Namespace, item.Spec.Storage.Size)
		*results = append(*results, ResourceMetrics{
			UID:          string(item.UID),
			APIVersion:   redis.GroupVersion.String(),
			Kind:         "Redis",
			Name:         item.Name,
			Namespace:    item.Namespace,
			Metrics:      m,
		})
	}
}

func (h *Handler) buildMetrics(ctx context.Context, now time.Time, name, namespace, storageSize string) map[string]any {
	m := map[string]any{
		"up":             0,
		"live_time_secs": 0,
		"storage_bytes":  func() int64 { q := resource.MustParse(storageSize); return q.Value() }(),
	}

	var podList corev1.PodList
	if err := h.client.List(ctx, &podList,
		client.InNamespace(namespace),
		client.MatchingLabels{"app": name},
	); err != nil || len(podList.Items) == 0 {
		return m
	}

	pod := podList.Items[0]
	if pod.Status.Phase != corev1.PodRunning {
		return m
	}

	m["up"] = 1
	if pod.Status.StartTime != nil {
		m["live_time_secs"] = int64(now.Sub(pod.Status.StartTime.Time).Seconds())
	}

	return m
}
