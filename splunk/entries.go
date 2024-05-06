package splunk

import "github.com/splunk/go-splunk-client/pkg/client"

type FeatureHealth struct {
	Health   string                   `json:"health"`
	Disabled bool                     `json:"disabled"` // if true, health is not relevant
	Features map[string]FeatureHealth `json:"features"`
}

type HealthSplunkdDetails struct {
	ID      client.ID     `selective:"create" service:"server/health/splunkd/details"`
	Content FeatureHealth `json:"content"`
}
