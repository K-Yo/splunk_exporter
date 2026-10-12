package splunk

import (
	"fmt"
)

// metricsQuery builds the query to get latest metrics values
func metricQuery(index string, metric string) string {
	return fmt.Sprintf(`
		| mstats
			latest(_value) as value
			where index="%s"
				  metric_name="%s"
			by metric_name [| mcatalog
				values(_dims) as dimensions
				where index="%s"
					  metric_name="%s"
				| eval search=mvjoin(dimensions, " ")
				| fields search]`,
		index, metric, index, metric)
}

// dimensionsQuery queries for dimensions names for one metric
func dimensionsQuery(index string, metric string) string {
	return fmt.Sprintf(`
		| mcatalog values(_dims) as dims
		  where index="%s" metric_name="%s"
		  by metric_name
		| fields dims
		| mvexpand dims`,
		index, metric)
}

// restQuery builds the query reading a REST endpoint on every member of a distributed search group.
// eai:* fields are dropped as some of them are multivalued, which search results decoding does not support.
func restQuery(serverGroup string, endpoint string) string {
	return fmt.Sprintf(`
		| rest splunk_server_group="%s" %s
		| fields - eai:*`,
		serverGroup, endpoint)
}
