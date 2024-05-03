package exporter

import (
	"fmt"
	"strings"

	"github.com/K-Yo/splunk_exporter/config"
	"github.com/K-Yo/splunk_exporter/splunk"
	splunklib "github.com/K-Yo/splunk_exporter/splunk"
	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/prometheus/client_golang/prometheus"
)

type MetricsManager struct {
	splunk    *splunklib.Splunk           // Splunk client
	namespace string                      // prometheus namespace for the metrics
	metrics   map[string]*prometheus.Desc // index format is index&metric_name
	logger    log.Logger
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

// ProcessMeasures will get all measures and send generated metrics in channel
// returns true if everything went well
func (mm *MetricsManager) ProcessMeasures(ch chan<- prometheus.Metric) bool {
	level.Info(mm.logger).Log("msg", "Getting custom measures")

	processMetricCallback := func(measure splunk.MetricMeasure, descriptor *prometheus.Desc) error {

		labelValues := make([]string, 0)
		for _, v := range measure.Labels {
			labelValues = append(labelValues, v)
		}
		ch <- prometheus.MustNewConstMetric(
			descriptor, prometheus.GaugeValue, measure.Value, labelValues...,
		)
		return nil
	}

	ret := true
	for key, _ := range mm.metrics {
		ret = ret && mm.ProcessOneMeasure(key, processMetricCallback)
	}

	level.Info(mm.logger).Log("msg", "Done getting custom measures", "success", ret)
	return ret
}

// ProcessOneMeasure gets a measure from splunk then calls the callback
func (mm *MetricsManager) ProcessOneMeasure(key string, callback func(splunk.MetricMeasure, *prometheus.Desc) error) bool {
	desc, ok := mm.metrics[key]
	if !ok {
		level.Error(mm.logger).Log("msg", "Unknown metric name", "name", key)
		return false
	}
	metric, index, err := mm.parseMetricKey(key)
	if err != nil {
		level.Error(mm.logger).Log("msg", "failed parsing a metric key", "key", key, "error", err)
	}

	cb := func(m splunklib.MetricMeasure) error {
		return callback(m, desc)
	}
	err = mm.splunk.GetMetricValues(index, metric, cb)

	if err != nil {
		level.Error(mm.logger).Log("msg", "Failed getting metric values", "err", err)
		return false
	} else {
		return true
	}
}

// getLabels retrieves Labels (Prometheus terminology, called dimensions in Splunk) for given metric
func (mm *MetricsManager) getLabels(metric config.Metric) []string {
	return mm.splunk.GetDimensions(metric.Index, metric.Name)
}

// normalizeName will format a splunk metric name so it can be accepted by prometheus
// FIXME update this method to match prometheus constraints
func (mm *MetricsManager) normalizeName(oldName string) string {
	newName := strings.ReplaceAll(oldName, ".", "_")
	level.Debug(mm.logger).Log("msg", "normalized metric name", "old", oldName, "new", newName)
	return newName
}

// parseMetricKey parses an internal metric key to get its name and index
func (mm *MetricsManager) parseMetricKey(key string) (metricName string, indexName string, err error) {
	err = nil
	if !strings.Contains(key, "&") {
		err = fmt.Errorf("key cannot be parsed, no char \"&\" found in it")
	}
	parts := strings.Split(key, "&")
	if !(len(parts) == 2) {
		err = fmt.Errorf("too many \"&\" in key: \"%s\"", key)
	}
	metricName = parts[0]
	indexName = parts[1]

	return
}

// newMetrics builds prom metrics for each of the settings configuration.
func newMetricsManager(conf []config.Metric, namespace string, splunk *splunklib.Splunk, logger log.Logger) *MetricsManager {
	metricsMap := make(map[string]*prometheus.Desc)
	mm := MetricsManager{
		splunk:    splunk,
		namespace: namespace,
		metrics:   metricsMap,
		logger:    logger,
	}

	for _, m := range conf {
		mm.Add(m)
	}

	return &mm
}
