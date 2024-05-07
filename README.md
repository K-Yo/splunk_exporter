# splunk_exporter

## Run

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

### from API

| Prefix                                                 | Description                                       |
| ------------------------------------------------------ | ------------------------------------------------- |
| `splunk_exporter_index_`                               | Numerical data coming from data/indexes endpoint. |
| `splunk_exporter_indexer_throughput_bytes_per_seconds` | average data throughput in indexer                |
| `splunk_exporter_metric_`                              | Export from metric indexes                        |

app doing searches? store them in kvstore?

- category
  - type[labelset]: value

- savedsearches
  - counter: ran
  - counter: skipped
  - histogram[status]: duration

metrics:
  list metrics name/value pairs: | mstats latest(*) where index=_* | rename latest(*) as *
  list metric names: | mcatalog values(metric_name) as metric_name WHERE index=_*  | mvexpand metric_name
  list dimensions when they exist: |mcatalog values(_dims) where index=metrics by metric_name

  get all values for one metric:

  | makeresults
| eval metric="test.metric.dimsbis"
| map search="| mstats latest(_value) as value where index=metrics metric_name=$metric$ by metric_name 
    [| mcatalog values(_dims) as dimensions where index=metrics metric_name=$metric$ 
| eval search=mvjoin(dimensions, \" \")
| fields search]"
  
  1. list metric names: https://docs.splunk.com/Documentation/Splunk/9.2.1/RESTREF/RESTmetrics#catalog.2Fmetricstore.2Fmetrics
  2. list dimensions: https://docs.splunk.com/Documentation/Splunk/9.2.1/RESTREF/RESTmetrics#catalog.2Fmetricstore.2Fdimensions
  3. 

  - scheduler lag: spl.mlog.searchscheduler.max_lag
  - skipped searches: spl.mlog.searchscheduler.skipped
  - events per index: spl.intr.disk_objects.Indexes.data.total_event_count


## TODO

- normalize data from splunk (metric name, labels names) such as removing dots (see specs)
- place all prometheus.Desc in a map indexed by metric name (before or after transform?)
- add to config the list of metrics to export (index + name)
- create an helper for logging
- cut http in smaller bits
- add API metrics (see old commits) because internal metrics are not reliable