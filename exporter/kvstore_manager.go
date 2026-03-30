package exporter

import (
	"fmt"
	"strconv"
	"strings"

	splunklib "github.com/K-Yo/splunk_exporter/splunk"
	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/prometheus/client_golang/prometheus"
)

type KVStoreManager struct {
	splunk           *splunklib.Splunk
	namespace        string
	logger           log.Logger
	statusDescriptor *prometheus.Desc
	memberStatusDesc *prometheus.Desc
	memberUptimeDesc *prometheus.Desc
}

func newKVStoreManager(namespace string, spk *splunklib.Splunk, logger log.Logger) *KVStoreManager {
	level.Debug(logger).Log("msg", "Initiating KVstore manager")

	km := KVStoreManager{
		splunk:    spk,
		namespace: namespace,
		logger:    logger,
		statusDescriptor: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "kvstore", "status"),
			"KVStore status from kvstore/status API",
			[]string{"server"}, nil,
		),
		memberStatusDesc: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "kvstore", "member_status"),
			"KVStore member replication status",
			[]string{"server", "member"}, nil,
		),
		memberUptimeDesc: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "kvstore", "member_uptime_seconds"),
			"KVStore member uptime in seconds",
			[]string{"server", "member"}, nil,
		),
	}

	level.Debug(logger).Log("msg", "Done initiating KVstore manager")
	return &km
}

func (km *KVStoreManager) CollectMeasures(ch chan<- prometheus.Metric) bool {
	level.Info(km.logger).Log("msg", "Collecting KVstore measures")

	// Read KVstore status
	kvstoreStatus := splunklib.KVStoreStatus{}
	if err := km.splunk.Client.Read(&kvstoreStatus); err != nil {
		level.Error(km.logger).Log("msg", "failed to read kvstore status", "err", err)
		return false
	}

	ret := true
	server := "local" // Default server identifier

	// Parse current.status (nested structure)
	if currentObj, ok := kvstoreStatus.Content["current"].(map[string]interface{}); ok {
		if currentStatus, ok := currentObj["status"].(string); ok {
			statusValue := km.currentStatusToFloat(currentStatus)
			ch <- prometheus.MustNewConstMetric(
				km.statusDescriptor, prometheus.GaugeValue, statusValue, server,
			)
			level.Debug(km.logger).Log("msg", "Collected KVstore status", "server", server, "status", currentStatus, "value", statusValue)
		} else {
			level.Warn(km.logger).Log("msg", "status field not found in current object")
			ret = false
		}
	} else {
		level.Warn(km.logger).Log("msg", "current object not found in kvstore/status response")
		ret = false
	}

	// Parse members (nested structure)
	members := km.parseMembersFromContent(kvstoreStatus.Content)
	for _, member := range members {
		// Member status metric
		statusValue := km.replicationStatusToFloat(member.ReplicationStatus)
		ch <- prometheus.MustNewConstMetric(
			km.memberStatusDesc, prometheus.GaugeValue, statusValue, server, member.HostAndPort,
		)
		level.Debug(km.logger).Log("msg", "Collected member status", "member", member.HostAndPort, "status", member.ReplicationStatus, "value", statusValue)

		// Member uptime metric
		ch <- prometheus.MustNewConstMetric(
			km.memberUptimeDesc, prometheus.GaugeValue, float64(member.Uptime), server, member.HostAndPort,
		)
		level.Debug(km.logger).Log("msg", "Collected member uptime", "member", member.HostAndPort, "uptime", member.Uptime)
	}

	level.Info(km.logger).Log("msg", "Done collecting KVstore measures", "members_count", len(members))
	return ret
}

// KVStoreMember represents one member of the KVstore replica set
type KVStoreMember struct {
	HostAndPort       string
	ReplicationStatus string
	Uptime            int
}

// parseMembersFromContent parses the nested members structure from the content map
func (km *KVStoreManager) parseMembersFromContent(content map[string]interface{}) []KVStoreMember {
	members := make([]KVStoreMember, 0)

	// Extract members object
	membersObj, ok := content["members"].(map[string]interface{})
	if !ok {
		level.Debug(km.logger).Log("msg", "members object not found or wrong type")
		return members
	}

	// Iterate through each member (keyed by index: "0", "1", "2", etc.)
	for memberKey, memberValue := range membersObj {
		fields, ok := memberValue.(map[string]interface{})
		if !ok {
			level.Warn(km.logger).Log("msg", "member value is not a map", "key", memberKey)
			continue
		}

		idx, _ := strconv.Atoi(memberKey) // For logging purposes
		member := KVStoreMember{}

		if hostAndPort, ok := fields["hostAndPort"].(string); ok {
			member.HostAndPort = hostAndPort
		} else {
			level.Warn(km.logger).Log("msg", "missing hostAndPort for member", "index", idx)
			continue
		}

		if replStatus, ok := fields["replicationStatus"].(string); ok {
			member.ReplicationStatus = replStatus
		} else {
			level.Warn(km.logger).Log("msg", "missing replicationStatus for member", "index", idx)
			member.ReplicationStatus = "Unknown status"
		}

		// Uptime can be int, float64, or string depending on XML/JSON parsing
		switch v := fields["uptime"].(type) {
		case float64:
			member.Uptime = int(v)
		case int:
			member.Uptime = v
		case string:
			if uptimeInt, err := strconv.Atoi(v); err == nil {
				member.Uptime = uptimeInt
			} else {
				level.Debug(km.logger).Log("msg", "failed to parse uptime string", "index", idx, "uptime", v)
				member.Uptime = 0
			}
		default:
			level.Debug(km.logger).Log("msg", "missing or invalid uptime for member", "index", idx, "type", fmt.Sprintf("%T", v))
			member.Uptime = 0
		}

		members = append(members, member)
	}

	return members
}

// currentStatusToFloat maps current.status to numeric values (case-insensitive)
func (km *KVStoreManager) currentStatusToFloat(status string) float64 {
	switch strings.ToLower(status) {
	case "ready":
		return 1
	case "starting":
		return 2
	case "shuttingdown":
		return 3
	case "disabled":
		return 4
	case "failed":
		return 5
	case "unknown":
		return 6
	default:
		level.Warn(km.logger).Log("msg", "unknown current.status value", "status", status)
		return 0
	}
}

// replicationStatusToFloat maps replicationStatus to numeric values (case-insensitive)
func (km *KVStoreManager) replicationStatusToFloat(status string) float64 {
	switch strings.ToLower(status) {
	case "kv store captain":
		return 1
	case "non-captain kv store member":
		return 2
	case "recovering":
		return 3
	case "initial sync":
		return 4
	case "startup":
		return 5
	case "down":
		return 6
	case "rollback":
		return 7
	case "removed":
		return 8
	case "unknown status":
		return 9
	default:
		level.Warn(km.logger).Log("msg", "unknown replicationStatus value", "status", status)
		return 0
	}
}
