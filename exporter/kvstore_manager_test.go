package exporter

import (
	"testing"

	splunklib "github.com/K-Yo/splunk_exporter/splunk"
	"github.com/go-kit/log"
	"github.com/prometheus/client_golang/prometheus"
)

func TestCurrentStatusToFloat(t *testing.T) {
	logger := log.NewNopLogger()
	km := newKVStoreManager("test", &splunklib.Splunk{}, logger)

	tests := []struct {
		status   string
		expected float64
	}{
		{"ready", 1},
		{"Ready", 1},        // Test case-insensitive
		{"READY", 1},        // Test case-insensitive
		{"starting", 2},
		{"Starting", 2},     // Test case-insensitive
		{"shuttingdown", 3},
		{"disabled", 4},
		{"failed", 5},
		{"unknown", 6},
		{"invalid", 0},
	}

	for _, tt := range tests {
		t.Run(tt.status, func(t *testing.T) {
			result := km.currentStatusToFloat(tt.status)
			if result != tt.expected {
				t.Errorf("currentStatusToFloat(%q) = %v, want %v", tt.status, result, tt.expected)
			}
		})
	}
}

func TestReplicationStatusToFloat(t *testing.T) {
	logger := log.NewNopLogger()
	km := newKVStoreManager("test", &splunklib.Splunk{}, logger)

	tests := []struct {
		status   string
		expected float64
	}{
		{"KV Store captain", 1},
		{"KV store captain", 1},              // Real Splunk API format
		{"kv store captain", 1},              // Test case-insensitive
		{"Non-captain KV Store member", 2},
		{"non-captain kv store member", 2},   // Test case-insensitive
		{"Recovering", 3},
		{"recovering", 3},                    // Test case-insensitive
		{"Initial Sync", 4},
		{"initial sync", 4},                  // Test case-insensitive
		{"Startup", 5},
		{"startup", 5},                       // Test case-insensitive
		{"Down", 6},
		{"down", 6},                          // Test case-insensitive
		{"Rollback", 7},
		{"Removed", 8},
		{"Unknown status", 9},
		{"invalid status", 0},
	}

	for _, tt := range tests {
		t.Run(tt.status, func(t *testing.T) {
			result := km.replicationStatusToFloat(tt.status)
			if result != tt.expected {
				t.Errorf("replicationStatusToFloat(%q) = %v, want %v", tt.status, result, tt.expected)
			}
		})
	}
}

func TestParseMembersFromContent(t *testing.T) {
	logger := log.NewNopLogger()
	km := newKVStoreManager("test", &splunklib.Splunk{}, logger)

	t.Run("parse multiple members", func(t *testing.T) {
		content := map[string]interface{}{
			"current": map[string]interface{}{
				"status": "ready",
			},
			"members": map[string]interface{}{
				"0": map[string]interface{}{
					"hostAndPort":       "sh1:8191",
					"replicationStatus": "KV Store captain",
					"uptime":            "86400",
				},
				"1": map[string]interface{}{
					"hostAndPort":       "sh2:8191",
					"replicationStatus": "Non-captain KV Store member",
					"uptime":            "43200",
				},
				"2": map[string]interface{}{
					"hostAndPort":       "sh3:8191",
					"replicationStatus": "Recovering",
					"uptime":            float64(3600),
				},
			},
		}

		members := km.parseMembersFromContent(content)

		if len(members) != 3 {
			t.Errorf("Expected 3 members, got %d", len(members))
		}

		// Verify first member (captain)
		found := false
		for _, m := range members {
			if m.HostAndPort == "sh1:8191" {
				found = true
				if m.ReplicationStatus != "KV Store captain" {
					t.Errorf("Expected captain status, got %s", m.ReplicationStatus)
				}
				if m.Uptime != 86400 {
					t.Errorf("Expected uptime 86400, got %d", m.Uptime)
				}
			}
		}
		if !found {
			t.Error("Captain member not found")
		}
	})

	t.Run("parse with missing fields", func(t *testing.T) {
		content := map[string]interface{}{
			"current": map[string]interface{}{
				"status": "ready",
			},
			"members": map[string]interface{}{
				"0": map[string]interface{}{
					"hostAndPort":       "sh1:8191",
					"replicationStatus": "KV Store captain",
					// missing uptime
				},
				"1": map[string]interface{}{
					"hostAndPort": "sh2:8191",
					// missing replicationStatus
					"uptime": float64(1000),
				},
			},
		}

		members := km.parseMembersFromContent(content)

		// Should still parse member 0, but member 1 might have defaults
		if len(members) == 0 {
			t.Error("Expected at least one member to be parsed")
		}
	})

	t.Run("empty content", func(t *testing.T) {
		content := map[string]interface{}{
			"current": map[string]interface{}{
				"status": "ready",
			},
		}

		members := km.parseMembersFromContent(content)

		if len(members) != 0 {
			t.Errorf("Expected 0 members, got %d", len(members))
		}
	})
}

func TestNewKVStoreManager(t *testing.T) {
	logger := log.NewNopLogger()
	spk := &splunklib.Splunk{}
	km := newKVStoreManager("test_namespace", spk, logger)

	if km == nil {
		t.Fatal("Expected KVStoreManager to be created, got nil")
	}

	if km.namespace != "test_namespace" {
		t.Errorf("Expected namespace 'test_namespace', got %s", km.namespace)
	}

	if km.statusDescriptor == nil {
		t.Error("Expected statusDescriptor to be initialized")
	}

	if km.memberStatusDesc == nil {
		t.Error("Expected memberStatusDesc to be initialized")
	}

	if km.memberUptimeDesc == nil {
		t.Error("Expected memberUptimeDesc to be initialized")
	}
}

// Mock collector for testing metrics output
type mockCollector struct {
	metrics []prometheus.Metric
}

func (m *mockCollector) Collect(metric prometheus.Metric) {
	m.metrics = append(m.metrics, metric)
}
