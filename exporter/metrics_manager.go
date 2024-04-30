package exporter

import (
	"fmt"
	"strings"

	"github.com/K-Yo/splunk_exporter/config"
	splunklib "github.com/K-Yo/splunk_exporter/splunk"
	"github.com/prometheus/client_golang/prometheus"
)

type MetricsManager struct {
	splunk    *splunklib.Splunk           // Splunk client
	namespace string                      // prometheus namespace for the metrics
	metrics   map[string]*prometheus.Desc // index format is index&metric_name
}

// up = prometheus.NewDesc(
// 	prometheus.BuildFQName(namespace, "", "up"),
// 	"Was the last query of Splunk successful.",
// 	nil, nil,
// )

// Add adds a new metric to the metrics manager from a configuration
func (mm *MetricsManager) Add(metric config.Metric) {
	key := fmt.Sprintf("%s&%s", metric.Index, metric.Name)
	name := mm.normalizeName(metric.Name)
	labels := mm.getLabels(metric)
	mm.metrics[key] = prometheus.NewDesc(
		prometheus.BuildFQName(mm.namespace, "", name),
		fmt.Sprintf("Splunk exported metric \"%s\" from index %s", metric.Name, metric.Index),
		labels, nil,
	)
}

// getLabels retrieves Labels (Prometheus terminology, called dimensions in Splunk) for given metric
func (mm *MetricsManager) getLabels(metric config.Metric) []string {
	return mm.splunk.GetDimensions(metric.Index, metric.Name)
}

func (mm *MetricsManager) normalizeName(oldName string) string {
	return strings.ReplaceAll(oldName, ".", "_")
}

// newMetrics builds prom metrics for each of the settings configuration.
func newMetricsManager(conf []config.Metric, namespace string, splunk *splunklib.Splunk) *MetricsManager {
	metricsMap := make(map[string]*prometheus.Desc)
	mm := MetricsManager{
		splunk:    splunk,
		namespace: namespace,
		metrics:   metricsMap,
	}

	for _, m := range conf {
		mm.Add(m)
	}

	return &mm
}
