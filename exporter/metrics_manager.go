package exporter

import (
	"fmt"
	"regexp"
	"slices"
	"strings"

	"github.com/K-Yo/splunk_exporter/config"
	splunklib "github.com/K-Yo/splunk_exporter/splunk"
	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/prometheus/client_golang/prometheus"
)

var (
	invalidPromNameChar = regexp.MustCompile(`[^a-zA-Z0-9_]`) // Regex to match a valid Prometheus Name
)

type Metric struct {
	Desc      *prometheus.Desc
	LabelsMap map[string]string //  key is splunk dimension, value is prom label. they are ordered.
}
type MetricsManager struct {
	splunk    *splunklib.Splunk // Splunk client
	namespace string            // prometheus namespace for the metrics
	metrics   map[string]Metric // index format is index&metric_name
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
	labelsMap, labelsPromNames := mm.getLabels(metric)
	mm.metrics[key] = Metric{
		LabelsMap: labelsMap,
		Desc: prometheus.NewDesc(
			prometheus.BuildFQName(mm.namespace, "", name),
			fmt.Sprintf("Splunk exported metric \"%s\" from index %s", metric.Name, metric.Index),
			labelsPromNames, nil,
		),
	}

}

// ProcessMeasures will get all measures and send generated metrics in channel
// returns true if everything went well
func (mm *MetricsManager) ProcessMeasures(ch chan<- prometheus.Metric) bool {
	level.Info(mm.logger).Log("msg", "Getting custom measures")

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

	ret := true
	for key := range mm.metrics {
		ret = ret && mm.ProcessOneMeasure(key, processMetricCallback)
	}

	level.Info(mm.logger).Log("msg", "Done getting custom measures", "success", ret)
	return ret
}

// ProcessOneMeasure gets a measure from splunk then calls the callback
func (mm *MetricsManager) ProcessOneMeasure(key string, callback func(splunklib.MetricMeasure, *prometheus.Desc) error) bool {
	metric, ok := mm.metrics[key]
	if !ok {
		level.Error(mm.logger).Log("msg", "Unknown metric name", "name", key)
		return false
	}
	metricName, index, err := mm.parseMetricKey(key)
	if err != nil {
		level.Error(mm.logger).Log("msg", "failed parsing a metric key", "key", key, "error", err)
	}

	cb := func(m splunklib.MetricMeasure) error {
		return callback(m, metric.Desc)
	}
	err = mm.splunk.GetMetricValues(index, metricName, cb)

	if err != nil {
		level.Error(mm.logger).Log("msg", "Failed getting metric values", "err", err)
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
func (mm *MetricsManager) getLabels(metric config.Metric) (map[string]string, []string) {
	labelsSplunkNames := mm.splunk.GetDimensions(metric.Index, metric.Name)
	level.Debug(mm.logger).Log("msg", "Retrieved labels for metric", "index", metric.Index, "metricName", metric.Name, "labels", strings.Join(labelsSplunkNames, ", "))
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
	indexName = parts[0]
	metricName = parts[1]
	level.Debug(mm.logger).Log("msg", "Parsed key into metric and index", "key", key, "metricName", metricName, "index", indexName)
	return
}

// newMetrics builds prom metrics for each of the settings configuration.
func newMetricsManager(conf []config.Metric, namespace string, splunk *splunklib.Splunk, logger log.Logger) *MetricsManager {
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

	return &mm
}
