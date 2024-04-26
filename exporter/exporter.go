package exporter

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/K-Yo/splunk_exporter/config"
	"github.com/K-Yo/splunk_exporter/splunk"
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
	average_input = prometheus.NewDesc(
		prometheus.BuildFQName(namespace, "", "input_bandwidth"),
		"What is the average input bandwidth",
		nil, nil,
	)
)

// Exporter collects Splunk stats from the given instance and exports them using the prometheus metrics package.
type Exporter struct {
	client *splunkclient.Client
	logger log.Logger
}

func (e *Exporter) UpdateConf(conf *config.Config) {
	// FIXME need to re-validate params
	e.client.TLSInsecureSkipVerify = conf.Insecure
	e.client.URL = conf.URL
	e.client.Authenticator = authenticators.Token{
		Token: conf.Token,
	}
}

type SplunkOpts struct {
	URI      string
	Token    string
	Insecure bool
}

// New creates a new exporter for Splunk metrics
func New(opts SplunkOpts, logger log.Logger) (*Exporter, error) {

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

	level.Info(logger).Log("msg", "Started Exporter", "instance", client.URL)

	return &Exporter{
		client: &client,
		logger: logger,
	}, nil
}

// Describe describes all the metrics ever exported by the Splunk exporter. It
// implements prometheus.Collector.
func (e *Exporter) Describe(ch chan<- *prometheus.Desc) {
	ch <- up
	ch <- average_input
}

// Collect fetches the stats from configured Splunk and delivers them
// as Prometheus metrics. It implements prometheus.Collector.
func (e *Exporter) Collect(ch chan<- prometheus.Metric) {
	ok := e.collectServicesMetric(ch)
	// ok = e.collectHealthStateMetric(ch) && ok
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

func (e *Exporter) collectServicesMetric(ch chan<- prometheus.Metric) bool {
	entry := &splunk.ServerIntrospectionIndexer{}
	err := e.client.Read(entry)
	if err != nil {
		level.Error(e.logger).Log("msg", "Could not read object in Splunk", "err", err)
		return false
	}
	level.Debug(e.logger).Log("msg", "Received metric", "namespace", entry.ID.Namespace, "average_KBps", entry.Content.AverageKBps)
	// value, err := strconv.ParseFloat(entry.Content.AverageKBps, 64)
	if err != nil {
		level.Error(e.logger).Log("msg", "Failed to parse metric as float", "namespace", entry.ID.Namespace, "name", "average_KBps", "value", entry.Content.AverageKBps)
		return false
	}
	ch <- prometheus.MustNewConstMetric(
		average_input, prometheus.GaugeValue, entry.Content.AverageKBps,
	)
	return true
}

// func (e *Exporter) collectHealthStateMetric(ch chan<- prometheus.Metric) bool {
// 	checks, _, err := e.client.Health().State("any", &e.queryOptions)
// 	if err != nil {
// 		level.Error(e.logger).Log("msg", "Failed to query service health", "err", err)
// 		return false
// 	}
// 	for _, hc := range checks {
// 		var passing, warning, critical, maintenance float64

// 		switch hc.Status {
// 		case consul_api.HealthPassing:
// 			passing = 1
// 		case consul_api.HealthWarning:
// 			warning = 1
// 		case consul_api.HealthCritical:
// 			critical = 1
// 		case consul_api.HealthMaint:
// 			maintenance = 1
// 		}

// 		if hc.ServiceID == "" {
// 			ch <- prometheus.MustNewConstMetric(
// 				nodeChecks, prometheus.GaugeValue, passing, hc.CheckID, hc.Node, consul_api.HealthPassing,
// 			)
// 			ch <- prometheus.MustNewConstMetric(
// 				nodeChecks, prometheus.GaugeValue, warning, hc.CheckID, hc.Node, consul_api.HealthWarning,
// 			)
// 			ch <- prometheus.MustNewConstMetric(
// 				nodeChecks, prometheus.GaugeValue, critical, hc.CheckID, hc.Node, consul_api.HealthCritical,
// 			)
// 			ch <- prometheus.MustNewConstMetric(
// 				nodeChecks, prometheus.GaugeValue, maintenance, hc.CheckID, hc.Node, consul_api.HealthMaint,
// 			)
// 		} else {
// 			ch <- prometheus.MustNewConstMetric(
// 				serviceChecks, prometheus.GaugeValue, passing, hc.CheckID, hc.Node, hc.ServiceID, hc.ServiceName, consul_api.HealthPassing,
// 			)
// 			ch <- prometheus.MustNewConstMetric(
// 				serviceChecks, prometheus.GaugeValue, warning, hc.CheckID, hc.Node, hc.ServiceID, hc.ServiceName, consul_api.HealthWarning,
// 			)
// 			ch <- prometheus.MustNewConstMetric(
// 				serviceChecks, prometheus.GaugeValue, critical, hc.CheckID, hc.Node, hc.ServiceID, hc.ServiceName, consul_api.HealthCritical,
// 			)
// 			ch <- prometheus.MustNewConstMetric(
// 				serviceChecks, prometheus.GaugeValue, maintenance, hc.CheckID, hc.Node, hc.ServiceID, hc.ServiceName, consul_api.HealthMaint,
// 			)
// 			ch <- prometheus.MustNewConstMetric(
// 				serviceCheckNames, prometheus.GaugeValue, 1, hc.ServiceID, hc.ServiceName, hc.CheckID, hc.Name, hc.Node,
// 			)
// 		}
// 	}
// 	return true
// }
