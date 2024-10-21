package exporter

import (
	"encoding/json"
	"fmt"
	"os"
	"testing"

	splunklib "github.com/K-Yo/splunk_exporter/splunk"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/go-kit/log"
	"github.com/stretchr/testify/assert"
)

func TestDeployment(t *testing.T) {
	_, w, _ := os.Pipe()
	defer w.Close()

	logger := log.NewJSONLogger(w)
	hm := HealthManager{
		logger:               logger,
		deploymentDescriptor: prometheus.NewDesc("metric", "", []string{"name", "instance_id"}, nil),
	}

	var deploymentHealth splunklib.HealthDeploymentDetails

	fileContent, err := os.ReadFile(`testdata/deploymenthealth.json`)
	assert.NoError(t, err)

	json.Unmarshal(fileContent, &deploymentHealth)

	ch := make(chan prometheus.Metric)

	// launch collector
	go func() {
		ret := hm.collectMetricsDeployment(
			ch,
			"",
			deploymentHealth.Content.Features,
		)
		close(ch)

		assert.True(t, ret)
	}()

	// empty chan
	go func() {
		for x := <-ch; x != nil; x = <-ch {
			fmt.Fprintln(os.Stdout, x)
		}
	}()

}
