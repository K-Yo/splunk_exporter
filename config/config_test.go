package config

import (
	"slices"
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

// TestLoadConfigSHCGroups
// Given
//
//	A valid config file with search head cluster groups, from both deprecated server_group and server_groups
//
// When
//
//	reloading the config
//
// Then
//
//	Every group is monitored once
func TestLoadConfigSHCGroups(t *testing.T) {
	sc := NewSafeConfig(prometheus.NewRegistry())

	err := sc.ReloadConfig("testdata/splunk_exporter-shc-groups.yml", nil)
	if err != nil {
		t.Fatalf("Error loading config %v: %v", "splunk_exporter-shc-groups.yml", err)
	}
	want := []string{"dmc_searchheadclustergroup_shc", "dmc_searchheadclustergroup_es_shc"}
	if got := sc.C.SHC.Groups(); !slices.Equal(got, want) {
		t.Errorf("SHC groups = %v, want %v", got, want)
	}
}
