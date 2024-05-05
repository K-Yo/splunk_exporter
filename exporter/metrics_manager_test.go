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

func TestNormalizeName(t *testing.T) {
	_, w, _ := os.Pipe()
	logger := log.NewJSONLogger(w)
	mm := MetricsManager{
		logger: logger,
	}

	n := mm.normalizeName("abc_@dèj:k*l__")

	assert.Equal(t, "abc__d_j_k_l__", n)

}
