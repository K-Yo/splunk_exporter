# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Overview

`splunk_exporter` is a Prometheus exporter that monitors a Splunk instance, written in Go. It queries the Splunk REST API (search, health, introspection endpoints) and exposes the results as Prometheus metrics.

## Common commands

```shell
# Build
go build -v ./...

# Run all tests
go test -v ./...

# Run a single package's tests
go test -v ./exporter/...

# Run a single test
go test -v ./exporter/ -run TestHealthManager_CollectMeasures

# Run locally against a config file
go run . --config.file=splunk_exporter_example.yml --log.level=debug
```

### Full local test bench (exporter + Splunk + Prometheus + Grafana via docker compose)

```shell
cd deploy/
bash run.sh          # starts everything
docker compose down  # stop it

# after making code changes, rebuild just the exporter container
docker compose up -d --build splunk_exporter
```

Service URLs are documented in `deploy/README.md` (Splunk on :8000, Grafana on :3000, Prometheus on :9090, exporter on :9115).

CI (`.github/workflows/ci.yml`) runs `go build -v ./...` and `go test -v ./...` on every push/PR to `main`. Releases publish a Docker image via `.github/workflows/build.yml`.

## Architecture

Request flow: `main.go` loads config → builds an `exporter.Exporter` → registers it with the Prometheus client library → serves `/metrics`. On SIGHUP, the config is reloaded and `Exporter.UpdateConf` rebuilds the underlying Splunk client (auth, TLS settings) without restarting the process.

Packages:

- **`config/`** — YAML config parsing (`splunk_exporter.yml` — see `splunk_exporter_example.yml` for the schema: `url`, `token` or `username`/`password`, `insecure`, `metrics[]`). `SafeConfig` guards the live config with an `RWMutex` so it can be hot-reloaded via SIGHUP or a `POST /-/reload` HTTP call while `Collect` is reading it concurrently.
- **`splunk/`** — thin client wrapping `github.com/splunk/go-splunk-client`. `splunk.go` builds and executes Splunk search queries (`Splunk.query`) and higher-level helpers (`GetMetricValues`, `GetDimensions`); `queries.go` holds the raw SPL search strings (`mstats`/`mcatalog`); `entries.go` defines REST API response structs for the health/introspection/indexes endpoints, tagged with `service:"..."` paths consumed by the Splunk client library.
- **`exporter/`** — implements `prometheus.Collector`. `exporter.go`'s `Collect` fans out to three independent metric sources every scrape, each best-effort (a failure in one doesn't stop the others; overall success feeds the `splunk_exporter_up` gauge):
  - `collectConfiguredMetrics` → `MetricsManager` (`metrics_manager.go`): user-configured metric-index queries from `config.Metrics`, one Splunk search per configured metric, with dynamic label sets derived from Splunk "dimensions".
  - `collectHealthMetrics` → `HealthManager` (`health_manager.go`): recursively walks the `server/health/splunkd/details` and `server/health/deployment/details` trees (green/yellow/red → 1.0/0.5/0.0), skipping `disabled` subtrees and synthetic `/instances` path segments.
  - `collectIndexerMetrics` (in `exporter.go`): `server/introspection/indexer` throughput + one gauge per index from `data/indexes`, with Prometheus metric descriptors created lazily and cached (`apiMetrics` map) since indexes/fields aren't known ahead of time.
- **`main.go`** — CLI flags (kingpin), HTTP endpoints (`/metrics`, `/config`, `/-/healthy`, `/-/reload`), and `web.external-url`/`route-prefix` handling shared with other Prometheus exporters.

All exported metrics are Gauges; see the table in `README.md` for the current metric name/label reference. Splunk field/dimension names are sanitized to valid Prometheus names via `invalidPromNameChar` regex (defined separately in both `exporter.go` and `metrics_manager.go`).

Because Splunk's schema for indexed metrics, health trees, and index fields is not known statically, most `prometheus.Desc` objects in this codebase are created lazily on first observation rather than declared upfront — `Exporter.Describe` intentionally emits nothing (this is an "unchecked" collector).
