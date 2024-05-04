package splunk

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	splunkclient "github.com/splunk/go-splunk-client/pkg/client"
)

type Splunk struct {
	Client *splunkclient.Client
	Logger log.Logger
}

type searchCallback func(data *SearchAPIResult, logger log.Logger) error

// GetDimensions returns the dimensions by alphabetical order for one metric
// it will return nil if no dimension exists
func (s *Splunk) GetDimensions(index string, metric string) []string {
	search := dimensionsQuery(index, metric)
	ch := make(chan string)

	callback := func(data *SearchAPIResult, logger log.Logger) error {
		for _, d := range data.Results {
			ch <- d["dims"]
		}
		close(ch)
		return nil
	}

	go func() {
		err := s.query(search, callback)
		if err != nil {
			level.Error(s.Logger).Log("msg", "failed to get dimensions", "err", err)
		}
	}()
	// get all dimensions
	ret := make([]string, 0)
	for d := range ch {
		ret = append(ret, d)
	}
	return ret
}

type MetricMeasure struct {
	Value  float64
	Labels map[string]string
}

// GetMetricValues retrieves all latest values for one metric on Splunk
// callback will be called on each measure
// errors on callback will be logged, and processing will continue
func (s *Splunk) GetMetricValues(index string, metric string, callback func(measure MetricMeasure) error) error {
	level.Debug(s.Logger).Log("msg", "Getting metric values", "index", index, "metric_name", metric)
	search := metricQuery(index, metric)
	queryCallback := func(data *SearchAPIResult, logger log.Logger) error {
		for _, m := range data.Results {
			name, ok := m["metric_name"]
			if !ok {
				level.Error(s.Logger).Log("msg", "could not find \"metric_name\" in splunk results.")
				// we ignore this result
				continue
			}
			delete(m, "metric_name")
			level.Debug(logger).Log("msg", "processing metric", "metric_name", name)
			value, ok := m["value"]
			if !ok {
				level.Error(s.Logger).Log("msg", "could not find \"value\" in splunk results.")
				// we ignore this result
				continue
			}
			fValue, err := strconv.ParseFloat(value, 64)
			if err != nil {
				level.Error(s.Logger).Log("msg", "Failed to parse value", "value", value, "err", err)
				// we ignore this result
				continue
			}
			delete(m, "value")

			measure := MetricMeasure{
				Value:  fValue,
				Labels: m,
			}
			if err := callback(measure); err != nil {
				level.Error(s.Logger).Log("msg", "Failed to run callback on measure", "measure", m)
			}
		}
		return nil
	}
	return s.query(search, queryCallback)
}

// query will search splunk
func (s *Splunk) query(search string, callbackFunc searchCallback) error {
	level.Debug(s.Logger).Log("msg", "performing Splunk query", "search", search)
	builder := func(req *http.Request) error {
		u, err := url.Parse(fmt.Sprintf("%s/%s", s.Client.URL, "services/search/v2/jobs"))
		if err != nil {
			return err
		}
		req.URL = u

		req.Method = http.MethodPost

		v := url.Values{}
		v.Set("exec_mode", "oneshot")
		v.Set("output_mode", "json")
		v.Set("search", search)
		req.Body = io.NopCloser(strings.NewReader(v.Encode()))

		err = s.Client.AuthenticateRequest(s.Client, req)
		if err != nil {
			return err
		}
		return nil
	}
	handler := func(resp *http.Response) error {
		var data SearchAPIResult
		if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
			level.Error(s.Logger).Log("msg", "could not decode payload", "err", err, "status", resp.Status)
			return err
		}
		level.Info(s.Logger).Log("msg", "received response from search, calling callback", "status", resp.Status, "num_results", len(data.Results))
		return callbackFunc(&data, s.Logger)
	}
	return s.Client.RequestAndHandle(builder, handler)
}
