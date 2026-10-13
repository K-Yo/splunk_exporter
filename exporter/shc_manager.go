package exporter

import (
	"fmt"
	"log/slog"
	"math"
	"strconv"
	"strings"
	"time"

	splunklib "github.com/K-Yo/splunk_exporter/splunk"
	"github.com/prometheus/client_golang/prometheus"
)

// Known states, so that every state of a state set is exported, even when no member is in it.
var (
	// shcMemberStatuses from shcluster/captain/members documentation
	shcMemberStatuses = []string{
		"Up", "Pending", "AutomaticDetention", "ManualDetention", "ManualDetention-PortsEnabled", "Restarting",
		"ShuttingDown", "ReassigningPrimaries", "Decommissioning", "GracefulShutdown", "Stopped", "Down", "BatchAdding",
	}
	// kvstoreStatuses from kvstore/status documentation (current.status)
	kvstoreStatuses = []string{"unknown", "disabled", "starting", "ready", "failed", "shuttingdown"}
	// kvstoreReplicationStatuses from kvstore/status documentation (members.<n>.replicationStatus)
	kvstoreReplicationStatuses = []string{
		"Startup", "KV store captain", "Non-captain KV store member", "Recovering", "Initial Sync",
		"Unknown status", "Down", "Rollback", "Removed",
	}
)

// SHCManager collects search head cluster and KV store replication metrics.
// As the exporter usually targets the Monitoring Console, endpoints are read on cluster members
// through the rest search command, using a distributed search group.
type SHCManager struct {
	splunk                       *splunklib.Splunk // Splunk client
	serverGroups                 []string          // distributed search groups, one per cluster, SHC monitoring is disabled when empty
	logger                       *slog.Logger
	memberStatusDesc             *prometheus.Desc
	memberOutOfSyncDesc          *prometheus.Desc
	memberLastConfReplication    *prometheus.Desc
	kvstoreStatusDesc            *prometheus.Desc
	kvstoreMemberReplicationDesc *prometheus.Desc
}

func newSHCManager(namespace string, serverGroups []string, spk *splunklib.Splunk, logger *slog.Logger) *SHCManager {

	logger.Debug("Initiating SHC manager")

	sm := SHCManager{
		splunk:       spk,
		serverGroups: serverGroups,
		logger:       logger,
		memberStatusDesc: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "shc", "member_status"),
			"Search head cluster member status, from shcluster/status API",
			[]string{"server_group", "server", "status"}, nil,
		),
		memberOutOfSyncDesc: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "shc", "member_out_of_sync"),
			"Whether search head cluster member configuration is out of sync with the captain, from shcluster/status API",
			[]string{"server_group", "server"}, nil,
		),
		memberLastConfReplication: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "shc", "member_last_conf_replication_timestamp_seconds"),
			"When search head cluster member last pulled configurations from the captain, from shcluster/status API",
			[]string{"server_group", "server"}, nil,
		),
		kvstoreStatusDesc: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "kvstore", "status"),
			"KV store status of search head cluster member, from kvstore/status API",
			[]string{"server_group", "server", "status"}, nil,
		),
		kvstoreMemberReplicationDesc: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "kvstore", "member_replication_status"),
			"KV store replica set member replication status, as seen by the KV store captain, from kvstore/status API",
			[]string{"server_group", "member", "status"}, nil,
		),
	}

	logger.Debug("Done initiating SHC manager")
	return &sm
}

// CollectMeasures will get all measures and send generated metrics in channel
// returns true if everything went well
func (sm *SHCManager) CollectMeasures(ch chan<- prometheus.Metric) bool {
	if len(sm.serverGroups) == 0 {
		sm.logger.Debug("No SHC server group configured, skipping SHC measures")
		return true
	}

	// clusters are independent, a failure on one does not prevent measuring the others
	ret := true
	for _, group := range sm.serverGroups {
		ret = sm.collectGroup(ch, group) && ret
	}
	return ret
}

// collectGroup measures the cluster whose members are in distributed search group
func (sm *SHCManager) collectGroup(ch chan<- prometheus.Metric, group string) bool {
	sm.logger.Info("Collecting SHC measures", "server_group", group)
	ret := true

	// KV store status is needed by cluster status, to get captain local time
	kvstoreResults, err := sm.splunk.GetRestResults(group, "/services/kvstore/status")
	if err != nil {
		sm.logger.Error("failed to read kvstore status", "server_group", group, "err", err)
		ret = false
	} else {
		ret = sm.measureKVStoreStatus(ch, group, kvstoreResults) && ret
	}

	clusterResults, err := sm.splunk.GetRestResults(group, "/services/shcluster/status advanced=1")
	if err != nil {
		sm.logger.Error("failed to read shcluster status", "server_group", group, "err", err)
		ret = false
	} else {
		ret = sm.measureClusterStatus(ch, group, clusterResults, kvstoreResults) && ret
	}

	sm.logger.Info("Done collecting SHC measures", "server_group", group, "success", ret)

	return ret
}

// measureClusterStatus measures cluster members from shcluster/status results
func (sm *SHCManager) measureClusterStatus(ch chan<- prometheus.Metric, group string, results []map[string]string, kvstoreResults []map[string]string) bool {
	// every member answers with the whole cluster view, prefer the one of the captain
	if len(results) == 0 {
		sm.logger.Error("no search head cluster member answered", "server_group", group)
		return false
	}
	view := results[0]
	for _, r := range results {
		if r["splunk_server"] == r["captain.label"] {
			view = r
			break
		}
	}

	// last_conf_replication is formatted in captain local time, without timezone
	captain := view["captain.label"]
	captainOffset, offsetErr := localTimeOffset(kvstoreResults, captain)
	if offsetErr != nil {
		sm.logger.Warn("cannot get captain local time offset, skipping last configuration replication", "server_group", group, "captain", captain, "err", offsetErr)
	}

	ret := true
	for guid, peer := range groupFields(view, "peers.") {
		server := peer["label"]
		if server == "" {
			server = guid
		}
		ret = sm.sendStateSet(ch, sm.memberStatusDesc, shcMemberStatuses, peer["status"], group, server) && ret

		outOfSync, err := flagToFloat(peer["out_of_sync_node"])
		if err != nil {
			sm.logger.Error("unexpected out_of_sync_node value", "server_group", group, "server", server, "err", err)
			ret = false
			continue
		}
		ch <- prometheus.MustNewConstMetric(
			sm.memberOutOfSyncDesc, prometheus.GaugeValue, outOfSync, group, server,
		)

		// captain has none, and it is "Pending" right after a captain election
		lastConfReplication := peer["last_conf_replication"]
		if offsetErr != nil || lastConfReplication == "" || lastConfReplication == "Pending" {
			continue
		}
		local, err := time.Parse(time.ANSIC, lastConfReplication)
		if err != nil {
			sm.logger.Error("unexpected last_conf_replication value", "server_group", group, "server", server, "err", err)
			ret = false
			continue
		}
		ch <- prometheus.MustNewConstMetric(
			sm.memberLastConfReplication, prometheus.GaugeValue, float64(local.Add(-captainOffset).Unix()), group, server,
		)
	}

	return ret
}

// localTimeOffset computes the UTC offset of server local time, from the date returned by kvstore/status
// both as local time (current.date) and epoch (current.dateSec).
func localTimeOffset(kvstoreResults []map[string]string, server string) (time.Duration, error) {
	for _, r := range kvstoreResults {
		if r["splunk_server"] != server {
			continue
		}
		// a server whose KV store is down answers without dates
		local, err := time.Parse(time.ANSIC, r["current.date"])
		if err != nil {
			return 0, fmt.Errorf("unexpected current.date value: %w", err)
		}
		epoch, err := strconv.ParseFloat(r["current.dateSec"], 64)
		if err != nil {
			return 0, fmt.Errorf("unexpected current.dateSec value: %w", err)
		}
		// current.date has no sub-second part, and offsets are whole minutes
		offset := math.Round((float64(local.Unix())-epoch)/60) * 60
		return time.Duration(offset) * time.Second, nil
	}
	return 0, fmt.Errorf("no kvstore status for server %q", server)
}

// measureKVStoreStatus measures the KV store of each cluster member from kvstore/status results
func (sm *SHCManager) measureKVStoreStatus(ch chan<- prometheus.Metric, group string, results []map[string]string) bool {
	ret := true
	var view map[string]string // replica set view, used for members replication status
	for _, r := range results {
		server := r["splunk_server"]
		ret = sm.sendStateSet(ch, sm.kvstoreStatusDesc, kvstoreStatuses, r["current.status"], group, server) && ret

		// a member whose KV store is down still answers, but with an outdated current.status
		// and no members: replication status must be read from another member, preferably the captain
		if _, ok := r["members.0.hostAndPort"]; ok && (view == nil || strings.EqualFold(r["current.replicationStatus"], "KV store captain")) {
			view = r
		}
	}

	if view == nil {
		sm.logger.Error("no KV store replica set view found", "server_group", group)
		return false
	}
	for _, member := range groupFields(view, "members.") {
		ret = sm.sendStateSet(ch, sm.kvstoreMemberReplicationDesc, kvstoreReplicationStatuses, member["replicationStatus"], group, member["hostAndPort"]) && ret
	}

	return ret
}

// sendStateSet sends one metric per known state for name in group: 1 for the current state, 0 for the others.
// Splunk casing is not consistent across versions, so states are matched case-insensitively.
// An unknown current state is sent as an additional state so it is not lost.
func (sm *SHCManager) sendStateSet(ch chan<- prometheus.Metric, desc *prometheus.Desc, states []string, current string, group string, name string) bool {
	if current == "" {
		sm.logger.Error("missing status", "server_group", group, "name", name, "desc", desc.String())
		return false
	}

	known := false
	for _, state := range states {
		value := 0.0
		if strings.EqualFold(state, current) {
			value = 1.0
			known = true
		}
		ch <- prometheus.MustNewConstMetric(desc, prometheus.GaugeValue, value, group, name, state)
	}
	if !known {
		sm.logger.Warn("unknown status", "server_group", group, "name", name, "status", current)
		ch <- prometheus.MustNewConstMetric(desc, prometheus.GaugeValue, 1.0, group, name, current)
	}
	return true
}

// groupFields groups flattened fields "<prefix><id>.<field>" by id, e.g. "peers.<guid>.label"
func groupFields(result map[string]string, prefix string) map[string]map[string]string {
	groups := make(map[string]map[string]string)
	for k, v := range result {
		rest, ok := strings.CutPrefix(k, prefix)
		if !ok {
			continue
		}
		id, field, ok := strings.Cut(rest, ".")
		if !ok {
			continue
		}
		if groups[id] == nil {
			groups[id] = make(map[string]string)
		}
		groups[id][field] = v
	}
	return groups
}

// flagToFloat parses a boolean flag as returned by the rest search command ("0" or "1")
func flagToFloat(flag string) (float64, error) {
	switch flag {
	case "0":
		return 0.0, nil
	case "1":
		return 1.0, nil
	default:
		return 0.0, fmt.Errorf("unknown flag value: %q", flag)
	}
}
