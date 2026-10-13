# splunk_exporter

[![Go Report Card](https://goreportcard.com/badge/github.com/K-Yo/splunk_exporter)](https://goreportcard.com/report/github.com/K-Yo/splunk_exporter)

Monitor a Splunk instance

## ❓ Howto

You will need a configuration file, follow [`splunk_exporter_example.yml`](./splunk_exporter_example.yml) format.

```
./splunk_exporter --help
```

## 🧪 Example run

You need docker compose installed, a bash helper is provided to start the exporter and the whole test bench as a [docker compose environment](./deploy/README.md).

```shell
cd deploy/
bash run.sh
```

To stop it:

```shell
docker compose down
```

## 👷 Contribute

After doing some changes, possible to re-deploy splunk_exporter with the following command
```shell
docker compose up -d --build splunk_exporter
```

## 🛠️ Configuration

Splunk exporter needs to access management APIs
See an example configuration file in [`splunk_exporter_example.yml`](./splunk_exporter_example.yml).

## 📏 metrics

All metrics are **Gauge**.

### from API

| Prefix                                                 | Labels                        | Description                                       |
| ------------------------------------------------------ | ----------------------------- | ------------------------------------------------- |
| `splunk_exporter_index_`                               | `index_name`                  | Numerical data coming from data/indexes endpoint. |
| `splunk_exporter_indexer_throughput_bytes_per_seconds` | _None_                        | Average data throughput in indexer                |
| `splunk_exporter_metric_`                              | Dimensions returned by Splunk | Export from metric indexes                        |
| `splunk_exporter_health_splunkd`                       | `name`                        | Health status from local splunkd                  |
| `splunk_exporter_health_deployment`                    | `instance_id`, `name`         | Health status from deployment                     |

### from search head cluster members

Only when `shc.server_groups` is configured: endpoints are read on every member of these distributed search groups (as defined on the Monitoring Console, one per cluster) with the `rest` search command.
Each cluster is measured independently, with its distributed search group as `server_group` label.
States are exported as state sets: one series per known state, `1` for the current state and `0` for the others.

| Metric                                                               | Labels                             | Description                                                                                                    |
| -------------------------------------------------------------------- | ---------------------------------- | -------------------------------------------------------------------------------------------------------------- |
| `splunk_exporter_shc_member_status`                                  | `server_group`, `server`, `status` | Member status (`Up`, `Down`, `ManualDetention`…) from shcluster/status                                         |
| `splunk_exporter_shc_member_out_of_sync`                             | `server_group`, `server`           | `1` when member configuration is out of sync with the captain (e.g. lost common baseline)                      |
| `splunk_exporter_shc_member_last_conf_replication_timestamp_seconds` | `server_group`, `server`           | When member last pulled configurations from the captain (captain local time converted using its KV store date) |
| `splunk_exporter_kvstore_status`                                     | `server_group`, `server`, `status` | KV store status (`ready`, `failed`…) as reported by member itself, from kvstore/status                         |
| `splunk_exporter_kvstore_member_replication_status`                  | `server_group`, `member`, `status` | KV store replica set member status (`Recovering`, `Down`…) as seen by KV store captain                         |

## 🚨 Alerting

Suggested Prometheus alerting rules for search head cluster and KV store replication, to adapt (thresholds, `for` durations, severities) to your environment.
`SplunkSHCMetricsMissing` only makes sense when `shc.server_groups` is configured; with several clusters, write one per group (e.g. `absent(splunk_exporter_shc_member_status{server_group="dmc_searchheadclustergroup_shc"})`), as `absent` cannot tell which one is missing.

Some notes on choices:
- members pull configurations from the captain every few seconds (`conf_replication_period`, 5s by default), so a 5 minutes old last pull means replication is stuck, even when the member is not (yet) flagged out of sync.
- a member whose KV store process died keeps reporting itself `ready`: KV store replication alerts rely on the KV store captain view (`splunk_exporter_kvstore_member_replication_status`).
- state sets contain every known state, so `{status="Up"} == 0` also catches unknown states.

```yaml
groups:
  - name: splunk_shc
    rules:
      - alert: SplunkExporterDown
        expr: splunk_exporter_up == 0
        for: 10m
        labels:
          severity: warning
        annotations:
          summary: "Splunk exporter cannot collect all metrics"
          description: "Some Splunk queries failed for 10 minutes, see exporter logs."

      - alert: SplunkSHCMetricsMissing
        expr: absent(splunk_exporter_shc_member_status)
        for: 10m
        labels:
          severity: warning
        annotations:
          summary: "No search head cluster metrics"
          description: "No member answered through the shc.server_groups distributed search groups for 10 minutes."

      - alert: SplunkSHCMemberOutOfSync
        expr: splunk_exporter_shc_member_out_of_sync == 1
        for: 5m
        labels:
          severity: critical
        annotations:
          summary: "SHC member {{ $labels.server }} configuration is out of sync"
          description: "{{ $labels.server }} lost its common baseline with the captain: changes do not replicate to or from it. Consider `splunk resync shcluster-replicated-config` on it."

      - alert: SplunkSHCConfReplicationStale
        expr: time() - splunk_exporter_shc_member_last_conf_replication_timestamp_seconds > 300
        for: 5m
        labels:
          severity: critical
        annotations:
          summary: "SHC member {{ $labels.server }} does not pull configurations from the captain"
          description: "{{ $labels.server }} last pulled configurations {{ $value | humanizeDuration }} ago (members pull every few seconds)."

      - alert: SplunkSHCMemberNotUp
        expr: splunk_exporter_shc_member_status{status="Up"} == 0
        for: 15m
        labels:
          severity: warning
        annotations:
          summary: "SHC member {{ $labels.server }} is not Up"
          description: "{{ $labels.server }} has not been Up for 15 minutes (down, in detention, restarting...)."

      - alert: SplunkKVStoreMemberNotReplicating
        expr: |
          sum by (server_group, member) (
            splunk_exporter_kvstore_member_replication_status{status=~"KV store captain|Non-captain KV store member"}
          ) == 0
        for: 5m
        labels:
          severity: critical
        annotations:
          summary: "KV store member {{ $labels.member }} is not replicating"
          description: "KV store captain sees {{ $labels.member }} neither as captain nor as member (recovering, down, rollback...)."

      - alert: SplunkKVStoreNotReady
        expr: splunk_exporter_kvstore_status{status="ready"} == 0
        for: 10m
        labels:
          severity: warning
        annotations:
          summary: "KV store of {{ $labels.server }} is not ready"
          description: "KV store of {{ $labels.server }} has not been ready for 10 minutes."
```

## 🧑‍🔬 Testing

```shell
go test -v ./...
```

## ✨ Roadmap

| Item                  | Status            |
| --------------------- | ----------------- |
| Metrics indexes       | ✅ Done            |
| Indexes metrics       | 🕰️ Ongoing         |
| SHC replication       | ✅ Done            |
| Savedsearches metrics | 🔜 Next            |
| System metrics        | ❓ Not planned yet |
| Ingestion pipeline    | ❓ Not planned yet |
