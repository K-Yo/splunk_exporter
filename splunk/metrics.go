package splunk

import "fmt"

func GetMetricQuery(index string, metric string) string {
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
