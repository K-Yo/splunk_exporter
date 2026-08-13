package exporter

import (
	"os"
	"testing"

	"github.com/go-kit/log"
	"github.com/stretchr/testify/assert"
)

// We shall test properly metrics being collected (from testdata/deploymenthealth.json for example).
// We’re waiting for https://github.com/prometheus/client_golang/issues/1639 to be resolved for this.

// New must return an error (not panic) when the configured Splunk URL is invalid,
// since getSplunkClient returns a nil client in that case.
func TestNew_InvalidSplunkURL(t *testing.T) {
	_, w, _ := os.Pipe()
	defer w.Close()
	logger := log.NewJSONLogger(w)

	exp, err := New(SplunkOpts{URI: ""}, logger, nil)

	assert.Error(t, err)
	assert.Nil(t, exp)
}
