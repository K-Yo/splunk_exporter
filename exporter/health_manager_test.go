package exporter

import (
	"encoding/json"
	"os"
	"testing"

	splunklib "github.com/K-Yo/splunk_exporter/splunk"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/go-kit/log"
	"github.com/stretchr/testify/assert"
)

func TestDeployment(t *testing.T) {
	_, w, _ := os.Pipe()
	logger := log.NewJSONLogger(w)
	hm := HealthManager{
		logger:               logger,
		deploymentDescriptor: prometheus.NewDesc("metric", "", []string{"name"}, nil),
	}

	var deploymentHealth splunklib.HealthDeploymentDetails

	fileContent, err := os.ReadFile(`testdata/deploymenthealth.json`)
	assert.NoError(t, err)

	json.Unmarshal(fileContent, &deploymentHealth)

	ret := hm.getMetricsDeployment(
		make(chan<- prometheus.Metric),
		"",
		deploymentHealth.Content.Features,
	)

	assert.True(t, ret)

}
