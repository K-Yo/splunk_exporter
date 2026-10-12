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
	"sync/atomic"
	"testing"

	splunklib "github.com/K-Yo/splunk_exporter/splunk"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/splunk/go-splunk-client/pkg/authenticators"
	splunkclient "github.com/splunk/go-splunk-client/pkg/client"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// testdata/shc_*.json are `| rest splunk_server_group=... | fields - eai:*` results captured from a
// Monitoring Console on a 3 members search head cluster (Splunk 10.4.0), where:
//   - SHC captain sh1 runs in America/New_York, KV store captain sh2 in Asia/Tokyo, sh3 in UTC
//   - sh3 lost its configuration baseline with the captain ("Error pulling configurations from captain")
//   - sh3 KV store (mongod) was killed
func newTestSHCManager(t *testing.T, serverGroup string, kvstoreResults func([]map[string]string) []map[string]string) (*SHCManager, *int32) {
	var calls int32
	fixture := func(name string) []map[string]string {
		content, err := os.ReadFile("testdata/" + name)
		require.NoError(t, err)
		var data splunklib.SearchAPIResult
		require.NoError(t, json.Unmarshal(content, &data))
		return data.Results
	}
	shcStatus := fixture("shc_status.json")
	kvstoreStatus := fixture("shc_kvstore_status.json")
	if kvstoreResults != nil {
		kvstoreStatus = kvstoreResults(kvstoreStatus)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		body, _ := io.ReadAll(r.Body)
		values, _ := url.ParseQuery(string(body))
		search := values.Get("search")

		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.Contains(search, "/services/shcluster/status"):
			json.NewEncoder(w).Encode(splunklib.SearchAPIResult{Results: shcStatus})
		case strings.Contains(search, "/services/kvstore/status"):
			json.NewEncoder(w).Encode(splunklib.SearchAPIResult{Results: kvstoreStatus})
		default:
			w.WriteHeader(http.StatusBadRequest)
		}
	}))
	t.Cleanup(server.Close)

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	client := &splunkclient.Client{
		URL:           server.URL,
		Authenticator: authenticators.Token{Token: "test"},
	}
	spk := &splunklib.Splunk{Client: client, Logger: logger}

	return newSHCManager("splunk_exporter", serverGroup, spk, logger), &calls
}

// collectorFunc adapts a collect function into an unchecked prometheus.Collector
type collectorFunc func(ch chan<- prometheus.Metric)

func (f collectorFunc) Describe(ch chan<- *prometheus.Desc) {}
func (f collectorFunc) Collect(ch chan<- prometheus.Metric) { f(ch) }

// collectSHC runs a collection and returns its success and measures, indexed by "name{label=value,...}"
// gathering fails on inconsistent or duplicated metrics.
func collectSHC(t *testing.T, sm *SHCManager) (bool, map[string]float64) {
	var ok bool
	reg := prometheus.NewPedanticRegistry()
	reg.MustRegister(collectorFunc(func(ch chan<- prometheus.Metric) { ok = sm.CollectMeasures(ch) }))
	families, err := reg.Gather()
	require.NoError(t, err)

	measures := make(map[string]float64)
	for _, family := range families {
		for _, m := range family.GetMetric() {
			labels := make([]string, 0)
			for _, l := range m.GetLabel() {
				labels = append(labels, l.GetName()+"="+l.GetValue())
			}
			measures[family.GetName()+"{"+strings.Join(labels, ",")+"}"] = m.GetGauge().GetValue()
		}
	}
	return ok, measures
}

func TestSHCManager_CollectMeasures(t *testing.T) {
	sm, _ := newTestSHCManager(t, "dmc_searchheadclustergroup_soc_shc", nil)

	ok, measures := collectSHC(t, sm)
	assert.True(t, ok)

	// configuration replication
	assert.Equal(t, 0.0, measures["splunk_exporter_shc_member_out_of_sync{server=sh1}"])
	assert.Equal(t, 0.0, measures["splunk_exporter_shc_member_out_of_sync{server=sh2}"])
	assert.Equal(t, 1.0, measures["splunk_exporter_shc_member_out_of_sync{server=sh3}"])

	// captain local time "Mon Oct 12 10:05:10 2026" is 14:05:10 UTC
	assert.Equal(t, 1791813910.0, measures["splunk_exporter_shc_member_last_conf_replication_timestamp_seconds{server=sh2}"])
	assert.Equal(t, 1791813665.0, measures["splunk_exporter_shc_member_last_conf_replication_timestamp_seconds{server=sh3}"])
	assert.NotContains(t, measures, "splunk_exporter_shc_member_last_conf_replication_timestamp_seconds{server=sh1}", "captain does not replicate from itself")

	// members status, every known state is exported
	assert.Equal(t, 1.0, measures["splunk_exporter_shc_member_status{server=sh3,status=Up}"])
	assert.Equal(t, 0.0, measures["splunk_exporter_shc_member_status{server=sh3,status=Down}"])

	// sh3 still reports its outdated KV store status
	assert.Equal(t, 1.0, measures["splunk_exporter_kvstore_status{server=sh3,status=ready}"])

	// but other members see it down
	assert.Equal(t, 1.0, measures["splunk_exporter_kvstore_member_replication_status{member=sh1:8191,status=Non-captain KV store member}"])
	assert.Equal(t, 1.0, measures["splunk_exporter_kvstore_member_replication_status{member=sh2:8191,status=KV store captain}"])
	assert.Equal(t, 1.0, measures["splunk_exporter_kvstore_member_replication_status{member=sh3:8191,status=Down}"])
	assert.Equal(t, 0.0, measures["splunk_exporter_kvstore_member_replication_status{member=sh3:8191,status=Recovering}"])

	assert.Len(t, measures,
		3*len(shcMemberStatuses)+3+2+3*len(kvstoreStatuses)+3*len(kvstoreReplicationStatuses),
		"replica set members must be exported once, from a single view")
}

// The KV store captain might not answer, replica set view must then be read from another member.
func TestSHCManager_KVStoreViewWithoutCaptain(t *testing.T) {
	withoutCaptain := func(results []map[string]string) []map[string]string {
		ret := make([]map[string]string, 0)
		for _, r := range results {
			if r["current.replicationStatus"] != "KV store captain" {
				ret = append(ret, r)
			}
		}
		return ret
	}
	sm, _ := newTestSHCManager(t, "dmc_searchheadclustergroup_soc_shc", withoutCaptain)

	ok, measures := collectSHC(t, sm)
	assert.True(t, ok)
	assert.Equal(t, 1.0, measures["splunk_exporter_kvstore_member_replication_status{member=sh3:8191,status=Down}"])
	assert.NotContains(t, measures, "splunk_exporter_kvstore_status{server=sh2,status=ready}")
}

// Without the SHC captain KV store date, its local time is unknown: last configuration replication
// cannot be converted, it must be skipped without failing the collection.
func TestSHCManager_CaptainLocalTimeUnknown(t *testing.T) {
	withoutCaptainDate := func(results []map[string]string) []map[string]string {
		for _, r := range results {
			if r["splunk_server"] == "sh1" {
				delete(r, "current.date")
			}
		}
		return results
	}
	sm, _ := newTestSHCManager(t, "dmc_searchheadclustergroup_soc_shc", withoutCaptainDate)

	ok, measures := collectSHC(t, sm)
	assert.True(t, ok)
	assert.NotContains(t, measures, "splunk_exporter_shc_member_last_conf_replication_timestamp_seconds{server=sh2}")
	assert.Equal(t, 1.0, measures["splunk_exporter_shc_member_out_of_sync{server=sh3}"])
}

func TestSHCManager_Disabled(t *testing.T) {
	sm, calls := newTestSHCManager(t, "", nil)

	ok, measures := collectSHC(t, sm)
	assert.True(t, ok)
	assert.Empty(t, measures)
	assert.Equal(t, int32(0), atomic.LoadInt32(calls), "Splunk must not be queried when SHC monitoring is disabled")
}

func TestSHCManager_UnknownStatus(t *testing.T) {
	unknownStatus := func(results []map[string]string) []map[string]string {
		for _, r := range results {
			if r["splunk_server"] == "sh2" {
				r["current.status"] = "brand new status"
			}
		}
		return results
	}
	sm, _ := newTestSHCManager(t, "dmc_searchheadclustergroup_soc_shc", unknownStatus)

	ok, measures := collectSHC(t, sm)
	assert.True(t, ok)
	assert.Equal(t, 1.0, measures["splunk_exporter_kvstore_status{server=sh2,status=brand new status}"])
	assert.Equal(t, 0.0, measures["splunk_exporter_kvstore_status{server=sh2,status=ready}"])
}
