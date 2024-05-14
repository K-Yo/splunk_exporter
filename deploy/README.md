# Test bench

This repository holds the files necessary to test splunk_exporter.

Through docker-compose, it starts the exporter, a prometheus instance, a grafana instance and a splunk instance.

## prerequisites

- bash
- docker-compose
- envsubst (part of `gettext` package)
- jq
- curl
- grep

## run

```shell
bash run.sh
```

## Access

All instances are exposed on your host.

This is not a production environment.

| Service         | URL                    | Credentials                       |
| --------------- | ---------------------- | --------------------------------- |
| Splunk          | http://localhost:8000/ | user: `admin` pass: `splunkadmin` |
| Grafana         | http://localhost:3000/ | user: `admin` pass: `admin`       |
| Prometheus      | http://localhost:9090/ |                                   |
| Splunk Exporter | http://localhost:9115/ |                                   |
