package exporter

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"github.com/K-Yo/splunk_exporter/config"
	"github.com/K-Yo/splunk_exporter/splunk"
	splunklib "github.com/K-Yo/splunk_exporter/splunk"
	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/splunk/go-splunk-client/pkg/authenticators"
	splunkclient "github.com/splunk/go-splunk-client/pkg/client"
)

const (
	namespace = "splunk_exporter"
)

var (
	up = prometheus.NewDesc(
		prometheus.BuildFQName(namespace, "", "up"),
		"Was the last query of Splunk successful.",
		nil, nil,
	)
	indexer_throughput = prometheus.NewDesc(
		prometheus.BuildFQName(namespace, "indexer", "throughput_bytes_per_seconds_average"),
		"Average throughput processed by instance indexer, from server/introspection/indexer endpoint",
		nil, nil,
	)
)

// Exporter collects Splunk stats from the given instance and exports them using the prometheus metrics package.
type Exporter struct {
	splunk         *splunk.Splunk
	logger         log.Logger
	indexedMetrics *MetricsManager
	healthMetrics  *HealthManager
	apiMetrics     map[string]*prometheus.Desc
}

func (e *Exporter) UpdateConf(conf *config.Config) {
	// FIXME need to re-validate params
	e.splunk.Client.TLSInsecureSkipVerify = conf.Insecure
	e.splunk.Client.URL = conf.URL
	e.splunk.Client.Authenticator = authenticators.Token{
		Token: conf.Token,
	}
}

type SplunkOpts struct {
	URI      string
	Token    string
	Insecure bool
}

// New creates a new exporter for Splunk metrics
func New(opts SplunkOpts, logger log.Logger, metricsConf []config.Metric) (*Exporter, error) {

	uri := opts.URI
	if !strings.Contains(uri, "://") {
		uri = "https://" + uri
	}
	u, err := url.Parse(uri)
	if err != nil {
		return nil, fmt.Errorf("invalid splunk URL: %s", err)
	}
	if u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
		return nil, fmt.Errorf("invalid splunk URL: %s", uri)
	}

	authenticator := authenticators.Token{
		Token: opts.Token,
	}
	client := splunkclient.Client{
		URL:                   opts.URI,
		Authenticator:         authenticator,
		TLSInsecureSkipVerify: opts.Insecure,
	}

	spk := splunk.Splunk{
		Client: &client,
		Logger: logger,
	}

	metricsManager := newMetricsManager(metricsConf, namespace, &spk, logger)
	healthManager := newHealthManager(namespace, &spk, logger)

	level.Info(logger).Log("msg", "Started Exporter", "instance", client.URL)

	return &Exporter{
		splunk:         &spk,
		logger:         logger,
		indexedMetrics: metricsManager,
		healthMetrics:  healthManager,
		apiMetrics:     make(map[string]*prometheus.Desc),
	}, nil
}

// Describe describes all the metrics ever exported by the Splunk exporter. It
// implements prometheus.Collector.
func (e *Exporter) Describe(ch chan<- *prometheus.Desc) {
	ch <- up
	for _, m := range e.indexedMetrics.metrics {
		ch <- m.Desc
	}
	for _, m := range e.apiMetrics {
		ch <- m
	}
}

// Collect fetches the stats from configured Splunk and delivers them
// as Prometheus metrics. It implements prometheus.Collector.
func (e *Exporter) Collect(ch chan<- prometheus.Metric) {
	ok := e.collectConfiguredMetrics(ch)
	ok = e.collectHealthMetrics(ch) && ok
	ok = e.collectIndexerMetrics(ch) && ok
	if ok {
		ch <- prometheus.MustNewConstMetric(
			up, prometheus.GaugeValue, 1.0,
		)
	} else {
		ch <- prometheus.MustNewConstMetric(
			up, prometheus.GaugeValue, 0.0,
		)
	}
}

// collectConfiguredMetrics gets metric measures from splunk indexes as specified by configuration
func (e *Exporter) collectConfiguredMetrics(ch chan<- prometheus.Metric) bool {

	return e.indexedMetrics.ProcessMeasures(ch)

}

// collectHealthMetrics grabs metrics from Splunk Health endpoints
func (e *Exporter) collectHealthMetrics(ch chan<- prometheus.Metric) bool {
	return e.healthMetrics.ProcessMeasures(ch)
}

func (e *Exporter) collectIndexerMetrics(ch chan<- prometheus.Metric) bool {
	ret := true
	level.Info(e.logger).Log("msg", "Collecting Indexer measures")
	introspectionIndexer := splunklib.ServerIntrospectionIndexer{}
	if err := e.splunk.Client.Read(&introspectionIndexer); err != nil {
		level.Error(e.logger).Log("msg", "failed to read indexer data", "err", err)
		ret = false
	}

	throughput := introspectionIndexer.Content.AverageKBps / 1000

	ch <- prometheus.MustNewConstMetric(
		indexer_throughput, prometheus.GaugeValue, throughput,
	)

	indexes := make([]splunklib.DataIndex, 0)
	if err := e.splunk.Client.List(&indexes); err != nil {
		level.Error(e.logger).Log("msg", "failed to list indexes", "err", err)
		ret = false
	}
	for _, i := range indexes {
		level.Debug(e.logger).Log("msg", "processing index", "index", i.ID.Title)
		ret = ret && e.measureIndex(ch, &i)
	}

	level.Info(e.logger).Log("msg", "Done collecting Indexer measures")
	return ret
}

// measureIndex returns measurements for one index, creating desc if they do not exist yet
func (e *Exporter) measureIndex(ch chan<- prometheus.Metric, index *splunklib.DataIndex) bool {
	ret := true
	indexName := index.ID.Title
	for typ, ival := range index.Content {
		var val float64
		var err error

		// FIXME add minDate and maxDate as titmestamps

		switch v := ival.(type) {
		case int:
			val = float64(v)
		case float64:
			val = v
		case string:
			val, err = strconv.ParseFloat(v, 64)
			if err != nil {
				continue
			}
		default:
			continue
		}

		name := e.normalizeName(typ)
		help := fmt.Sprintf("Index %s from Splunk data/indexes API", typ)
		e.CreateIfNeededThenMeasure(ch, "index", name, help, val, []string{"index_name"}, []string{indexName})
	}
	return ret
}

// normalizeName will format a string so it can be accepted by prometheus as a metric name or label
// see https://prometheus.io/docs/concepts/data_model/#metric-names-and-labels
func (e *Exporter) normalizeName(oldName string) string {
	newName := invalidPromNameChar.ReplaceAllString(oldName, "_")
	level.Debug(e.logger).Log("msg", "normalized name", "old", oldName, "new", newName)
	return newName
}

// CreateIfNeededThenMeasure Measures a metric, and creates it if it does not exist yet in local registry
func (e *Exporter) CreateIfNeededThenMeasure(
	ch chan<- prometheus.Metric,
	subsystem string,
	name string,
	help string,
	value float64,
	labels []string,
	labelValues []string) {

	metricFQName := prometheus.BuildFQName(namespace, subsystem, name)
	descriptor, exists := e.apiMetrics[metricFQName]
	if !exists {
		descriptor = prometheus.NewDesc(
			metricFQName,
			help,
			labels,
			nil,
		)
		e.apiMetrics[metricFQName] = descriptor
	}

	// measure
	ch <- prometheus.MustNewConstMetric(
		descriptor, prometheus.GaugeValue, value, labelValues...,
	)
}
