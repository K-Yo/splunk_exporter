package exporter

import (
	"fmt"
	"log/slog"
	"regexp"
	"slices"
	"strings"
	"sync"

	"github.com/K-Yo/splunk_exporter/config"
	splunklib "github.com/K-Yo/splunk_exporter/splunk"
	"github.com/prometheus/client_golang/prometheus"
)

var (
	invalidPromNameChar = regexp.MustCompile(`[^a-zA-Z0-9_]`) // Regex to match a valid Prometheus Name
)

type Metric struct {
	Name      string
	Index     string
	Desc      *prometheus.Desc
	LabelsMap map[string]string //  key is splunk dimension, value is prom label. they are ordered.
}
type MetricsManager struct {
	splunk    *splunklib.Splunk // Splunk client
	namespace string            // prometheus namespace for the metrics
	metrics   map[string]Metric // index format is index&metric_name
	metricsMu sync.Mutex        // guards metrics
	logger    *slog.Logger
}

// Add adds a new metric to the metrics manager from a configuration
func (mm *MetricsManager) Add(metric config.Metric) {

	mm.logger.Debug("Registering metric", "namespace", "metrics", "name", metric.Name, "index", metric.Index)

	key := fmt.Sprintf("%s&%s", metric.Index, metric.Name)

	mm.metricsMu.Lock()
	mm.metrics[key] = Metric{
		Name:  metric.Name,
		Index: metric.Index,
	}
	mm.metricsMu.Unlock()
}

// CollectMeasures will get all measures and send generated metrics in channel
// returns true if everything went well
func (mm *MetricsManager) CollectMeasures(ch chan<- prometheus.Metric) bool {
	mm.logger.Info("Getting custom measures")

	processMetricCallback := func(measure splunklib.MetricMeasure, descriptor *prometheus.Desc) error {

		labelValues := make([]string, 0)
		labelKeys := make([]string, 0)
		for k := range measure.Labels {
			labelKeys = append(labelKeys, k)
		}
		slices.Sort(labelKeys)
		for _, k := range labelKeys {
			labelValues = append(labelValues, measure.Labels[k])
		}
		ch <- prometheus.MustNewConstMetric(
			descriptor, prometheus.GaugeValue, measure.Value, labelValues...,
		)
		return nil
	}

	mm.metricsMu.Lock()
	keys := make([]string, 0, len(mm.metrics))
	for key := range mm.metrics {
		keys = append(keys, key)
	}
	mm.metricsMu.Unlock()

	ret := true
	for _, key := range keys {
		ret = ret && mm.ProcessOneMeasure(key, processMetricCallback)
	}

	mm.logger.Info("Done getting custom measures", "success", ret)
	return ret
}

// ProcessOneMeasure gets a measure from splunk then calls the callback
func (mm *MetricsManager) ProcessOneMeasure(key string, callback func(splunklib.MetricMeasure, *prometheus.Desc) error) bool {
	mm.metricsMu.Lock()
	metric, ok := mm.metrics[key]
	if !ok {
		mm.metricsMu.Unlock()
		mm.logger.Error("Unknown metric name, this should not happen", "name", key)
		return false
	}
	if metric.Desc == nil {
		mm.logger.Debug("First time seeing this metric, will create desc for it.", "name", key)

		name := mm.normalizeName(metric.Name)
		labelsMap, labelsPromNames := mm.getLabels(metric)
		metric.Desc = prometheus.NewDesc(
			prometheus.BuildFQName(mm.namespace, "metric", name),
			fmt.Sprintf("Splunk exported metric \"%s\" from index %s", metric.Name, metric.Index),
			labelsPromNames, nil,
		)
		metric.LabelsMap = labelsMap
		mm.metrics[key] = metric
	}
	mm.metricsMu.Unlock()

	metricName, index, err := mm.parseMetricKey(key)
	if err != nil {
		mm.logger.Error("failed parsing a metric key", "key", key, "error", err)
	}

	cb := func(m splunklib.MetricMeasure) error {
		return callback(m, metric.Desc)
	}
	err = mm.splunk.GetMetricValues(index, metricName, cb)

	if err != nil {
		mm.logger.Error("Failed getting metric values", "err", err)
		return false
	} else {
		return true
	}
}

// getLabels retrieves Labels (Prometheus terminology, called dimensions in Splunk) for given metric
// it then creates a map to rename labels according to prometheus rules
// it returns two items
//   - a map whose keys are label names and values label values
//   - a slice of label values (ordered after the map keys)
func (mm *MetricsManager) getLabels(metric Metric) (map[string]string, []string) {
	labelsSplunkNames := mm.splunk.GetDimensions(metric.Index, metric.Name)
	mm.logger.Debug("Retrieved labels for metric", "index", metric.Index, "metricName", metric.Name, "labels", strings.Join(labelsSplunkNames, ", "))
	labelsMap := make(map[string]string)
	labelsPromNames := make([]string, 0)
	slices.Sort(labelsSplunkNames)
	for _, labelSplunkName := range labelsSplunkNames {
		labelPromName := mm.normalizeName(labelSplunkName)
		labelsMap[labelSplunkName] = labelPromName
		labelsPromNames = append(labelsPromNames, labelPromName)
	}
	return labelsMap, labelsPromNames
}

// normalizeName will format a splunk metric name (or any other name) so it can be accepted by prometheus
// see https://prometheus.io/docs/concepts/data_model/#metric-names-and-labels
func (mm *MetricsManager) normalizeName(oldName string) string {
	newName := invalidPromNameChar.ReplaceAllString(oldName, "_")
	mm.logger.Debug("normalized metric name", "old", oldName, "new", newName)
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
	indexName = parts[0]
	metricName = parts[1]
	mm.logger.Debug("Parsed key into metric and index", "key", key, "metricName", metricName, "index", indexName)
	return
}

// newMetrics builds prom metrics for each of the settings configuration.
func newMetricsManager(conf []config.Metric, namespace string, splunk *splunklib.Splunk, logger *slog.Logger) *MetricsManager {

	logger.Debug("Initiating metrics manager")

	metricsMap := make(map[string]Metric)
	mm := MetricsManager{
		splunk:    splunk,
		namespace: namespace,
		metrics:   metricsMap,
		logger:    logger,
	}

	for _, m := range conf {
		mm.Add(m)
	}

	logger.Debug("Done initiating metrics manager")

	return &mm
}
