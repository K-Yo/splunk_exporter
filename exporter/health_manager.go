package exporter

import (
	"fmt"
	"strings"

	splunklib "github.com/K-Yo/splunk_exporter/splunk"
	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/prometheus/client_golang/prometheus"
)

type HealthManager struct {
	splunk               *splunklib.Splunk // Splunk client
	namespace            string            // prometheus namespace for the metrics
	logger               log.Logger
	splunkdDescriptor    *prometheus.Desc
	deploymentDescriptor *prometheus.Desc
}

func newHealthManager(namespace string, spk *splunklib.Splunk, logger log.Logger) *HealthManager {

	level.Debug(logger).Log("msg", "Initiating health manager")

	hm := HealthManager{
		splunk:    spk,
		namespace: namespace,
		logger:    logger,
		splunkdDescriptor: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "health", "splunkd"),
			"Splunk exported metric from splunkd health API",
			[]string{"name"}, nil,
		),
		deploymentDescriptor: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "health", "deployment"),
			"Splunk exported metric from deployment health API",
			[]string{"name", "instance_id"}, nil,
		),
	}

	level.Debug(logger).Log("msg", "Done initiating health manager")
	return &hm
}

func (hm *HealthManager) ProcessMeasures(ch chan<- prometheus.Metric) bool {

	// collect splunkd health metrics
	level.Info(hm.logger).Log("msg", "Collecting Splunkd Health measures")
	splunkdHealth := splunklib.HealthSplunkdDetails{}
	if err := hm.splunk.Client.Read(&splunkdHealth); err != nil {
		level.Error(hm.logger).Log("msg", "failed to read health data", "err", err)
		return false
	}

	ret := hm.getMetricsSplunkd(ch, "", &splunkdHealth.Content)

	level.Info(hm.logger).Log("msg", "Done collecting Splunkd Health measures")

	// collect deployment health metrics

	level.Info(hm.logger).Log("msg", "Collecting Deployment Health measures")

	deploymentHealth := splunklib.HealthDeploymentDetails{}
	if err := hm.splunk.Client.Read(&deploymentHealth); err != nil {
		level.Error(hm.logger).Log("msg", "failed to read health data", "err", err)
		return false
	}

	ret = ret && hm.getMetricsDeployment(ch, "", deploymentHealth.Content.Features)
	level.Info(hm.logger).Log("msg", "Done collecting Deployment Health measures")

	return ret
}

// getMetricsSplunkd recursively get all metric measures from a health endpoint result and sends them in the channel
// disabled features are not measured
func (hm *HealthManager) getMetricsSplunkd(ch chan<- prometheus.Metric, path string, fh *splunklib.FeatureHealth) bool {
	ret := true
	if !fh.Disabled {
		healthValue, err := hm.healthToFloat(fh.Health)
		if err != nil {
			level.Error(hm.logger).Log("msg", "Cannot get metrics because of health value", "path", path, "err", err)
			ret = false
		}
		displayPath := path
		if path == "" {
			displayPath = "/"
		}
		ch <- prometheus.MustNewConstMetric(
			hm.splunkdDescriptor, prometheus.GaugeValue, healthValue, displayPath,
		)
	}

	for name, child := range fh.Features {
		ret = ret && hm.getMetricsSplunkd(ch, fmt.Sprintf("%s/%s", path, name), &child)
	}

	return ret

}

// getMetricsDeployment recursively get all metric measures from a health endpoint result and sends them in the channel
// disabled features are not measured
func (hm *HealthManager) getMetricsDeployment(ch chan<- prometheus.Metric, path string, data map[string]interface{}) bool {
	level.Debug(hm.logger).Log("msg", "Getting Deployment metrics", "path", path)
	ret := true
	var disabled bool = false
	var health string      // health of current level
	var num_red float64    // num_red for current level
	var num_yellow float64 // num_yellow for current level

	for key, ival := range data {
		level.Debug(hm.logger).Log("msg", "Processing", "key", key, "path", path)

		switch key {
		case "health":
			health = ival.(string)
			continue
		case "num_red":
			num_red = ival.(float64)
			continue
		case "num_yellow":
			num_yellow = ival.(float64)
			continue
		case "disabled":
			disabled = ival.(bool)
			continue
		case "eai:acl":
			continue
		}

		switch v := ival.(type) {
		case map[string]interface{}:
			newPath := fmt.Sprintf("%s/%s", path, key)
			// recursively get lower level metrics
			ret = ret && hm.getMetricsDeployment(ch, newPath, v)
		default:
			level.Error(hm.logger).Log("msg", "unknown type for key", "key", key, "path", path)
		}
	}
	level.Debug(hm.logger).Log("num_red", num_red, "num_yellow", num_yellow)

	skipMetric := false
	level.Debug(hm.logger).Log("path", path, "lenpath", len(path))
	skipMetric = skipMetric || disabled     // ignore disabled metrics
	skipMetric = skipMetric || health == "" // ignore when we cannot parse health

	// ignore when it’s a metric ending with "/instances"
	if len(path) > 10 {
		skipMetric = skipMetric || (path[len(path)-10:] == "/instances")
	}
	// Add current metric
	if !skipMetric {
		healthValue, err := hm.healthToFloat(health)
		if err != nil {
			level.Error(hm.logger).Log("msg", "Cannot get metrics because of unknown health value", "path", path, "err", err)
			ret = false
		}
		displayPath := path
		if path == "" {
			displayPath = "/"
		}

		// process when metrics are in the form of /splunkd/resource_usage/iowait/sum_top3_cpu_percs__max_last_3m/instances/8F8096AF-A456-4974-92FB-966103FA9752
		instanceId := ""
		if strings.Contains(path, "/instances/") {
			parts := strings.Split(path, "/")
			if parts[len(parts)-2] == "instances" {
				instanceId = parts[len(parts)-1]
			}
			displayPath = strings.Join(parts[:len(parts)-2], "/")
		}

		ch <- prometheus.MustNewConstMetric(
			hm.deploymentDescriptor, prometheus.GaugeValue, healthValue, displayPath, instanceId,
		)
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
