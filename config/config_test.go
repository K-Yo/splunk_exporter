package config

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus"
)

func TestLoadConfig(t *testing.T) {
	sc := NewSafeConfig(prometheus.NewRegistry())

	err := sc.ReloadConfig("testdata/splunk_exporter-good.yml", nil)
	if err != nil {
		t.Errorf("Error loading config %v: %v", "splunk_exporter-good.yml", err)
	}
}
