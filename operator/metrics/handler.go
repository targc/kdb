package metrics

import (
	"context"
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"kdb.io/operator/mongo"
	"kdb.io/operator/postgres"
	"kdb.io/operator/redis"
)

var labels = []string{"uid", "api_version", "kind", "name", "namespace"}

var (
	up = prometheus.NewDesc("kdb_up", "Whether the resource pod is running (1=up, 0=down)", labels, nil)
	liveTimeSecs = prometheus.NewDesc("kdb_live_time_seconds", "Seconds since pod started", labels, nil)
	storageMB = prometheus.NewDesc("kdb_storage_mb", "Provisioned storage in megabytes", labels, nil)
)

type Collector struct {
	client client.Client
}

func NewCollector(c client.Client) *Collector {
	return &Collector{client: c}
}

func (c *Collector) Describe(ch chan<- *prometheus.Desc) {
	ch <- up
	ch <- liveTimeSecs
	ch <- storageMB
}

func (c *Collector) Collect(ch chan<- prometheus.Metric) {
	ctx := context.Background()
	now := time.Now()

	c.collectPostgres(ctx, now, ch)
	c.collectMongo(ctx, now, ch)
	c.collectRedis(ctx, now, ch)
}

func (c *Collector) collectPostgres(ctx context.Context, now time.Time, ch chan<- prometheus.Metric) {
	var list postgres.PostgresList
	if err := c.client.List(ctx, &list); err != nil {
		return
	}
	for _, item := range list.Items {
		lv := []string{string(item.UID), postgres.GroupVersion.String(), "Postgres", item.Name, item.Namespace}
		c.emitMetrics(ctx, now, ch, lv, item.Name, item.Namespace, item.Spec.Storage.Size)
	}
}

func (c *Collector) collectMongo(ctx context.Context, now time.Time, ch chan<- prometheus.Metric) {
	var list mongo.MongoList
	if err := c.client.List(ctx, &list); err != nil {
		return
	}
	for _, item := range list.Items {
		lv := []string{string(item.UID), mongo.GroupVersion.String(), "Mongo", item.Name, item.Namespace}
		c.emitMetrics(ctx, now, ch, lv, item.Name, item.Namespace, item.Spec.Storage.Size)
	}
}

func (c *Collector) collectRedis(ctx context.Context, now time.Time, ch chan<- prometheus.Metric) {
	var list redis.RedisList
	if err := c.client.List(ctx, &list); err != nil {
		return
	}
	for _, item := range list.Items {
		lv := []string{string(item.UID), redis.GroupVersion.String(), "Redis", item.Name, item.Namespace}
		c.emitMetrics(ctx, now, ch, lv, item.Name, item.Namespace, item.Spec.Storage.Size)
	}
}

func (c *Collector) emitMetrics(ctx context.Context, now time.Time, ch chan<- prometheus.Metric, lv []string, name, namespace, storageSize string) {
	q := resource.MustParse(storageSize)
	ch <- prometheus.MustNewConstMetric(storageMB, prometheus.GaugeValue, float64(q.Value()/(1024*1024)), lv...)

	var podList corev1.PodList
	if err := c.client.List(ctx, &podList,
		client.InNamespace(namespace),
		client.MatchingLabels{"app": name},
	); err != nil || len(podList.Items) == 0 {
		ch <- prometheus.MustNewConstMetric(up, prometheus.GaugeValue, 0, lv...)
		ch <- prometheus.MustNewConstMetric(liveTimeSecs, prometheus.GaugeValue, 0, lv...)
		return
	}

	pod := podList.Items[0]
	if pod.Status.Phase != corev1.PodRunning {
		ch <- prometheus.MustNewConstMetric(up, prometheus.GaugeValue, 0, lv...)
		ch <- prometheus.MustNewConstMetric(liveTimeSecs, prometheus.GaugeValue, 0, lv...)
		return
	}

	ch <- prometheus.MustNewConstMetric(up, prometheus.GaugeValue, 1, lv...)
	var live int64
	if pod.Status.StartTime != nil {
		live = int64(now.Sub(pod.Status.StartTime.Time).Seconds())
	}
	ch <- prometheus.MustNewConstMetric(liveTimeSecs, prometheus.GaugeValue, float64(live), lv...)
}

func NewHTTPHandler(c client.Client) http.Handler {
	collector := NewCollector(c)
	reg := prometheus.NewRegistry()
	reg.MustRegister(collector)
	return promhttp.HandlerFor(reg, promhttp.HandlerOpts{})
}
