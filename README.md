# splunk_exporter

[![Go Report Card](https://goreportcard.com/badge/github.com/K-Yo/splunk_exporter)](https://goreportcard.com/report/github.com/K-Yo/splunk_exporter)

## Howto

You will need a configuration file, follow [`splunk_exporter_example.yml`](./splunk_exporter_example.yml) format.

```
./splunk_exporter --help
```

## Example run

You need docker compose installed, a bash helper is provided to start the exporter and the whole test bench as a [docker compose environment](./deploy/README.md).

```shell
cd /deploy
bash run.sh
```

To stop it:

```shell
docker compose down
```

## Contribute

After doing some changes, possible to re-deploy splunk_exporter with the following command
```shell
docker compose up -d --build splunk_exporter
```

## metrics

All metrics are **Gauge**.

### from API

| Prefix                                                 | Description                                       |
| ------------------------------------------------------ | ------------------------------------------------- |
| `splunk_exporter_index_`                               | Numerical data coming from data/indexes endpoint. |
| `splunk_exporter_indexer_throughput_bytes_per_seconds` | average data throughput in indexer                |
| `splunk_exporter_metric_`                              | Export from metric indexes                        |
