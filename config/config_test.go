package config

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus"
)

// TestLoadConfigToken
// Given
//
//	A valid config file using a token
//
// When
//
//	reloading the config
//
// Then
//
//	Config reload happens without error
func TestLoadConfigToken(t *testing.T) {
	sc := NewSafeConfig(prometheus.NewRegistry())

	err := sc.ReloadConfig("testdata/splunk_exporter-token-good.yml", nil)
	if err != nil {
		t.Errorf("Error loading config %v: %v", "splunk_exporter-good.yml", err)
	}
}

// TestLoadConfigUser
// Given
//
//	A valid config file using a username and password
//
// When
//
//	reloading the config
//
// Then
//
//	Config reload happens without error
func TestLoadConfigUser(t *testing.T) {
	sc := NewSafeConfig(prometheus.NewRegistry())

	err := sc.ReloadConfig("testdata/splunk_exporter-user-good.yml", nil)
	if err != nil {
		t.Errorf("Error loading config %v: %v", "splunk_exporter-good.yml", err)
	}
}
