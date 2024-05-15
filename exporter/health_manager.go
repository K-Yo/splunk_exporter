package exporter

import (
	"fmt"

	splunklib "github.com/K-Yo/splunk_exporter/splunk"
	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/prometheus/client_golang/prometheus"
)

type HealthManager struct {
	splunk            *splunklib.Splunk // Splunk client
	namespace         string            // prometheus namespace for the metrics
	logger            log.Logger
	splunkdDescriptor *prometheus.Desc
	descriptors       map[string]*prometheus.Desc
}

func newHealthManager(namespace string, spk *splunklib.Splunk, logger log.Logger) *HealthManager {

	level.Debug(logger).Log("msg", "Initiating health manager")

	descriptors := make(map[string]*prometheus.Desc)
	hm := HealthManager{
		splunk:    spk,
		namespace: namespace,
		logger:    logger,
		splunkdDescriptor: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "", "health"),
			"Splunk exported metric from splunkd health API",
			[]string{"name"}, nil,
		),
		descriptors: descriptors,
	}

	level.Debug(logger).Log("msg", "Done initiating health manager")
	return &hm
}

func (hm *HealthManager) ProcessMeasures(ch chan<- prometheus.Metric) bool {
	level.Info(hm.logger).Log("msg", "Collecting Health measures")
	splunkdHealth := splunklib.HealthSplunkdDetails{}
	if err := hm.splunk.Client.Read(&splunkdHealth); err != nil {
		level.Error(hm.logger).Log("msg", "failed to read health data", "err", err)
		return false
	}

	level.Info(hm.logger).Log("msg", "Done collecting Health measures")

	return hm.getMetrics(ch, "", &splunkdHealth.Content)
}

// getMetrics recursively get all metric measures from a health endpoint result and sends them in the channel
// disabled features are not measured
func (hm *HealthManager) getMetrics(ch chan<- prometheus.Metric, path string, fh *splunklib.FeatureHealth) bool {
	ret := true
	if !fh.Disabled {
		healthValue, err := hm.healthToFloat(fh.Health)
		if err != nil {
			level.Error(hm.logger).Log("msg", "Cannot get metrics because of health value", "path", path, "err", err)
			ret = false
		}
		if path == "" {
			path = "/"
		}
		ch <- prometheus.MustNewConstMetric(
			hm.splunkdDescriptor, prometheus.GaugeValue, healthValue, path,
		)
	}

	for name, child := range fh.Features {
		ret = ret && hm.getMetrics(ch, fmt.Sprintf("%s/%s", path, name), &child)
	}

	return ret

}

// healthToFloat retrieves a metric value from the "green"/"yellow"/"red" returned by Splunk
func (hm *HealthManager) healthToFloat(health string) (float64, error) {
	if health == "green" {
		return 1.0, nil
	} else if health == "yellow" {
		return 0.5, nil
	} else if health == "red" {
		return 0.0, nil
	} else {
		return 0.0, fmt.Errorf("unknown health value: %s", health)
	}
}
