package exporter

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
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
	total_event_count = prometheus.NewDesc(
		prometheus.BuildFQName(namespace, "", "total_event_count"),
		"total_event_count",
		[]string{"data_name", "component", "log_level"}, nil,
	)
)

// Exporter collects Splunk stats from the given instance and exports them using the prometheus metrics package.
type Exporter struct {
	client  *splunkclient.Client
	logger  log.Logger
	metrics map[string]*prometheus.Desc
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
	ch <- total_event_count
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
	index := "_metrics"
	metric := "spl.intr.disk_objects.Indexes.data.total_event_count"
	builder := func(req *http.Request) error {
		u, err := url.Parse(fmt.Sprintf("%s/%s", e.client.URL, "services/search/v2/jobs"))
		if err != nil {
			return err
		}
		req.URL = u

		req.Method = http.MethodPost

		v := url.Values{}
		v.Set("exec_mode", "oneshot")
		v.Set("output_mode", "json")
		v.Set("search", splunk.GetMetricQuery(index, metric))
		req.Body = io.NopCloser(strings.NewReader(v.Encode()))

		err = e.client.AuthenticateRequest(e.client, req)
		if err != nil {
			return err
		}
		return nil
	}
	handler := func(resp *http.Response) error {
		var data splunk.APIResult
		if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
			level.Error(e.logger).Log("msg", "could not decode payload", "err", err, "status", resp.Status)
			return err
		}
		level.Info(e.logger).Log("msg", "received response from search", "status", resp.Status, "num_results", len(data.Results))
		for _, m := range data.Results {
			name, ok := m["metric_name"]
			if !ok {
				// we ignore this result
				continue
			}
			delete(m, "metric_name")
			level.Info(e.logger).Log("msg", "processing metric", "metric_name", name)
			value, ok := m["value"]
			if !ok {
				// we ignore this result
				continue
			}
			fValue, err := strconv.ParseFloat(value, 64)
			if err != nil {
				continue
			}
			delete(m, "value")
			labelValues := make([]string, 0)
			for _, k := range []string{"data.name", "component", "log_level"} {
				labelValues = append(labelValues, m[k])
			}
			ch <- prometheus.MustNewConstMetric(
				total_event_count, prometheus.GaugeValue, fValue, labelValues...,
			)
		}
		return nil
	}
	e.client.RequestAndHandle(builder, handler)
	// TODO
	// ch <- prometheus.MustNewConstMetric(
	// 	average_input, prometheus.GaugeValue, entry.Content.AverageKBps,
	// )
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
