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
| `splunk_exporter_kvstore_status`                       | `server`                      | KVStore status (0=invalid, 1=ready, 2=starting, 3=shuttingdown, 4=disabled, 5=failed, 6=unknown) |
| `splunk_exporter_kvstore_member_status`                | `server`, `member`            | KVStore member replication status (0=invalid, 1=captain, 2=member, 3=recovering, 4=initial sync, 5=startup, 6=down, 7=rollback, 8=removed, 9=unknown) |
| `splunk_exporter_kvstore_member_uptime_seconds`        | `server`, `member`            | KVStore member uptime in seconds                  |

## 🧑‍🔬 Testing

```shell
go test -v ./...
```

## ✨ Roadmap

| Item                  | Status            |
| --------------------- | ----------------- |
| Metrics indexes       | ✅ Done            |
| Indexes metrics       | 🕰️ Ongoing         |
| KVStore metrics       | ✅ Done            |
| Savedsearches metrics | 🔜 Next            |
| System metrics        | ❓ Not planned yet |
| Ingestion pipeline    | ❓ Not planned yet |
