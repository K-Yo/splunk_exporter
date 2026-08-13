package exporter

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/K-Yo/splunk_exporter/config"
	splunklib "github.com/K-Yo/splunk_exporter/splunk"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/splunk/go-splunk-client/pkg/authenticators"
	splunkclient "github.com/splunk/go-splunk-client/pkg/client"
	"github.com/stretchr/testify/assert"
)

func TestParseMetricKey(t *testing.T) {
	_, w, _ := os.Pipe()
	logger := slog.New(slog.NewTextHandler(w, nil))
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
	logger := slog.New(slog.NewTextHandler(w, nil))
	mm := MetricsManager{
		logger: logger,
	}

	n := mm.normalizeName("abc_@dèj:k*l__")

	assert.Equal(t, "abc__d_j_k_l__", n)

}

// TestProcessOneMeasure_CachesDimensions reproduces the scenario where the
// per-metric Desc/dimensions built on first use were never written back into
// the metrics map, so every scrape re-fetched dimensions from Splunk instead
// of using the cached value.
func TestProcessOneMeasure_CachesDimensions(t *testing.T) {
	var dimensionCalls int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		values, _ := url.ParseQuery(string(body))
		search := values.Get("search")

		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(search, "mvexpand") {
			atomic.AddInt32(&dimensionCalls, 1)
			json.NewEncoder(w).Encode(splunklib.SearchAPIResult{
				Results: []map[string]string{{"dims": "host"}},
			})
			return
		}
		json.NewEncoder(w).Encode(splunklib.SearchAPIResult{
			Results: []map[string]string{{"metric_name": "some.metric", "value": "1.0", "host": "server1"}},
		})
	}))
	defer server.Close()

	_, w, _ := os.Pipe()
	logger := slog.New(slog.NewTextHandler(w, nil))

	client := &splunkclient.Client{
		URL:           server.URL,
		Authenticator: authenticators.Token{Token: "test"},
	}
	spk := &splunklib.Splunk{Client: client, Logger: logger}

	mm := newMetricsManager([]config.Metric{{Name: "some.metric", Index: "main"}}, "splunk_exporter", spk, logger)

	key := "main&some.metric"
	callback := func(m splunklib.MetricMeasure, d *prometheus.Desc) error { return nil }

	assert.True(t, mm.ProcessOneMeasure(key, callback))
	assert.True(t, mm.ProcessOneMeasure(key, callback))

	assert.Equal(t, int32(1), atomic.LoadInt32(&dimensionCalls), "dimensions should be fetched once and then cached across scrapes")
	assert.NotNil(t, mm.metrics[key].Desc, "Desc built on first use must be persisted back into the metrics map")
}

// mm.metrics is ranged over by CollectMeasures while ProcessOneMeasure reads
// and writes it (caching the Desc/dimensions on first use), with no
// synchronization between the two. Concurrent scrapes race on it.
// Run with `go test -race` to observe the data race.
func TestMetricsManager_ConcurrentAccess(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		values, _ := url.ParseQuery(string(body))
		search := values.Get("search")

		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(search, "mvexpand") {
			json.NewEncoder(w).Encode(splunklib.SearchAPIResult{
				Results: []map[string]string{{"dims": "host"}},
			})
			return
		}
		json.NewEncoder(w).Encode(splunklib.SearchAPIResult{
			Results: []map[string]string{{"metric_name": "some.metric", "value": "1.0", "host": "server1"}},
		})
	}))
	defer server.Close()

	_, w, _ := os.Pipe()
	defer w.Close()
	logger := slog.New(slog.NewTextHandler(w, nil))

	client := &splunkclient.Client{
		URL:           server.URL,
		Authenticator: authenticators.Token{Token: "test"},
	}
	spk := &splunklib.Splunk{Client: client, Logger: logger}

	mm := newMetricsManager([]config.Metric{{Name: "some.metric", Index: "main"}}, "splunk_exporter", spk, logger)
	key := "main&some.metric"
	callback := func(m splunklib.MetricMeasure, d *prometheus.Desc) error { return nil }

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

	for round := 0; round < 50; round++ {
		var wg sync.WaitGroup
		wg.Add(2)
		go func() {
			defer wg.Done()
			mm.ProcessOneMeasure(key, callback)
		}()
		go func() {
			defer wg.Done()
			mm.CollectMeasures(ch)
		}()
		wg.Wait()

		// force the "first time seeing this metric" race window again next round
		m := mm.metrics[key]
		m.Desc = nil
		mm.metrics[key] = m
	}
	close(done)
}
