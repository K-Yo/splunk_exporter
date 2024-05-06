package exporter

import (
	"fmt"
	"net/url"
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
	splunk  *splunk.Splunk
	logger  log.Logger
	metrics *MetricsManager
	hm      *HealthManager
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

	level.Info(logger).Log("msg", "Started Exporter", "instance", client.URL)

	return &Exporter{
		splunk:  &spk,
		logger:  logger,
		metrics: newMetricsManager(metricsConf, namespace, &spk, logger),
		hm:      newHealthManager(namespace, &spk, logger),
	}, nil
}

// Describe describes all the metrics ever exported by the Splunk exporter. It
// implements prometheus.Collector.
func (e *Exporter) Describe(ch chan<- *prometheus.Desc) {
	ch <- up
	for _, m := range e.metrics.metrics {
		ch <- m.Desc
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

	return e.metrics.ProcessMeasures(ch)

}

// collectHealthMetrics grabs metrics from Splunk Health endpoints
func (e *Exporter) collectHealthMetrics(ch chan<- prometheus.Metric) bool {
	return e.hm.ProcessMeasures(ch)
}

func (e *Exporter) collectIndexerMetrics(ch chan<- prometheus.Metric) bool {
	level.Info(e.logger).Log("msg", "Collecting Indexer measures")
	introspectionIndexer := splunklib.ServerIntrospectionIndexer{}
	if err := e.splunk.Client.Read(&introspectionIndexer); err != nil {
		level.Error(e.logger).Log("msg", "failed to read indexer data", "err", err)
		return false
	}

	throughput := introspectionIndexer.Content.AverageKBps / 1000

	ch <- prometheus.MustNewConstMetric(
		indexer_throughput, prometheus.GaugeValue, throughput,
	)

	level.Info(e.logger).Log("msg", "Done collecting Indexer measures")
	return true
}
