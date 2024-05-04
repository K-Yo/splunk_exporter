package exporter

import (
	"os"
	"testing"

	"github.com/go-kit/log"
	"github.com/stretchr/testify/assert"
)

func TestParseMetricKey(t *testing.T) {
	_, w, _ := os.Pipe()
	logger := log.NewJSONLogger(w)
	mm := MetricsManager{
		logger: logger,
	}

	metric, index, err := mm.parseMetricKey("index&metric.name")

	assert.NoError(t, err)
	assert.Equal(t, "metric.name", metric)
	assert.Equal(t, "index", index)

}
