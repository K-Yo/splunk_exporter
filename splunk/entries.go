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

type ServerIntrospectionIndexerContent struct {
	AverageKBps float64 `json:"average_KBps"` // Average indexer throughput (kbps).
	Reason      string  `json:"reason"`       // Status explanation. For a normal status, returns `.` . The following examples show possible abnormal status reasons.
	Status      string  `json:"status"`       // Current indexer status. One of the following values. normal, throttled, stopped
}

// ServerIntrospectionIndexer https://docs.splunk.com/Documentation/Splunk/9.2.1/RESTREF/RESTintrospect#server.2Fintrospection.2Findexer
type ServerIntrospectionIndexer struct {
	ID      client.ID                         `selective:"create" service:"server/introspection/indexer"`
	Content ServerIntrospectionIndexerContent `json:"content"`
}

// DataIndex https://docs.splunk.com/Documentation/Splunk/9.2.1/RESTREF/RESTintrospect#data.2Findexes
type DataIndex struct {
	ID      client.ID              `selective:"create" service:"data/indexes"`
	Content map[string]interface{} `json:"content"`
}
