package exporter

import (
	"os"
	"sync"
	"testing"

	"github.com/K-Yo/splunk_exporter/config"
	"github.com/go-kit/log"
	"github.com/prometheus/client_golang/prometheus"
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

// UpdateConf (triggered from the SIGHUP goroutine) mutates the shared
// splunk client's URL/Authenticator/TLSInsecureSkipVerify fields in place
// while Collect (triggered from an HTTP scrape goroutine) reads those same
// fields. Run with `go test -race` to observe the data race.
func TestExporter_ConcurrentUpdateConfAndCollect(t *testing.T) {
	_, w, _ := os.Pipe()
	defer w.Close()
	logger := log.NewJSONLogger(w)

	// unroutable-but-immediately-refused address: fails fast, no real network needed.
	exp, err := New(SplunkOpts{URI: "http://127.0.0.1:1"}, logger, nil)
	if err != nil {
		t.Fatalf("failed to build exporter: %v", err)
	}

	ch := make(chan prometheus.Metric, 1000)
	done := make(chan struct{})
	go func() {
		for {
			select {
			case <-ch:
			case <-done:
				return
			}
		}
	}()

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		for i := 0; i < 50; i++ {
			exp.Collect(ch)
		}
	}()
	go func() {
		defer wg.Done()
		for i := 0; i < 50; i++ {
			exp.UpdateConf(&config.Config{URL: "http://127.0.0.1:1", Insecure: i%2 == 0})
		}
	}()
	wg.Wait()
	close(done)
}

// apiMetrics is a plain map read and written from CreateIfNeededThenMeasure
// with no synchronization; concurrent scrapes race on it. Run with
// `go test -race` to observe the data race.
func TestExporter_ConcurrentCreateIfNeededThenMeasure(t *testing.T) {
	_, w, _ := os.Pipe()
	defer w.Close()
	logger := log.NewJSONLogger(w)

	exp := &Exporter{
		logger:     logger,
		apiMetrics: make(map[string]*prometheus.Desc),
	}

	ch := make(chan prometheus.Metric, 100)
	done := make(chan struct{})
	go func() {
		for {
			select {
			case <-ch:
			case <-done:
				return
			}
		}
	}()

	for round := 0; round < 200; round++ {
		var wg sync.WaitGroup
		for i := 0; i < 4; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				exp.CreateIfNeededThenMeasure(ch, "index", "foo", "help", 1.0, []string{"index_name"}, []string{"main"})
			}()
		}
		wg.Wait()
		delete(exp.apiMetrics, prometheus.BuildFQName(namespace, "index", "foo"))
	}
	close(done)
}
