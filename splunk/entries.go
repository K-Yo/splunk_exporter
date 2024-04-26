package splunk

import (
	"github.com/splunk/go-splunk-client/pkg/client"
)

// RoleContent defines the Content for a Role.
type ServerIntrospectionIndexerContent struct {
	AverageKBps string `json:"average_KBps" values:"-"`
}

// Role defines a Splunk role.
type ServerIntrospectionIndexer struct {
	ID      client.ID                         `selective:"create" service:"server/introspection/indexer"`
	Content ServerIntrospectionIndexerContent `json:"content"`
}
