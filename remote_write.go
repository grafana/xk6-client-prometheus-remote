package remotewrite

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"math"
	"math/rand"
	"net/http"
	"net/url"
	"strconv"
	"sync"
	"time"

	"github.com/golang/protobuf/proto"
	"github.com/golang/snappy"
	"github.com/pkg/errors"
	"github.com/prometheus/prometheus/prompb"
	"github.com/xhit/go-str2duration/v2"
	"go.k6.io/k6/js/common"
	"go.k6.io/k6/js/modules"
	"go.k6.io/k6/lib"
	"go.k6.io/k6/lib/netext/httpext"
)

var timeSeriesPool sync.Pool

// Register the extension on module initialization, available to
// import from JS as "k6/x/remotewrite".
func init() {
	modules.Register("k6/x/remotewrite", new(RemoteWrite))
	timeSeriesPool = sync.Pool{
		New: func() interface{} {
			return []prompb.TimeSeries{}
		},
	}
}

// RemoteWrite is the k6 extension for interacting Prometheus Remote Write endpoints.
type RemoteWrite struct{}

// Client is the client wrapper.
type Client struct {
	client *http.Client
	cfg    *Config

	preGeneratedLabelSets [][]prompb.Label
}

type Config struct {
	Url        string `json:"url"`
	UserAgent  string `json:"user_agent"`
	Timeout    string `json:"timeout"`
	TenantName string `json:"tenant_name"`
}

// XClient represents
func (r *RemoteWrite) XClient(ctxPtr *context.Context, config Config) interface{} {
	if config.Url == "" {
		log.Fatal(fmt.Errorf("url is required"))
	}
	if config.UserAgent == "" {
		config.UserAgent = "k6-remote-write/0.0.2"
	}
	if config.Timeout == "" {
		config.Timeout = "10s"
	}

	rt := common.GetRuntime(*ctxPtr)
	return common.Bind(rt, &Client{
		client: &http.Client{},
		cfg:    &config,
	}, ctxPtr)
}

type Timeseries struct {
	Labels  []Label
	Samples []Sample
}

type Label struct {
	Name, Value string
}

type Sample struct {
	Value     float64
	Timestamp int64
}

func (r *RemoteWrite) XSample(value float64, timestamp int64) Sample {
	return Sample{
		Value:     value,
		Timestamp: timestamp,
	}
}

func (r *RemoteWrite) XTimeseries(labels map[string]string, samples []Sample) *Timeseries {
	t := &Timeseries{
		Labels:  make([]Label, 0, len(labels)),
		Samples: samples,
	}

	for k, v := range labels {
		t.Labels = append(t.Labels, Label{Name: k, Value: v})
	}

	return t
}

func (r *RemoteWrite) XPromPbLabel(name, value string) prompb.Label {
	return prompb.Label{
		Name:  name,
		Value: value,
	}
}

func (c *Client) Store(ctx context.Context, ts []Timeseries) (httpext.Response, error) {
	var batch []prompb.TimeSeries
	for _, t := range ts {
		batch = append(batch, FromTimeseriesToPrometheusTimeseries(t))
	}

	// Required for k6 metrics
	state := lib.GetState(ctx)
	if state == nil {
		return *httpext.NewResponse(ctx), errors.New("State is nil")
	}

	req := prompb.WriteRequest{
		Timeseries: batch,
	}

	data, err := proto.Marshal(&req)
	if err != nil {
		return *httpext.NewResponse(ctx), errors.Wrap(err, "failed to marshal remote-write request")
	}

	compressed := snappy.Encode(nil, data)

	res, err := c.send(ctx, state, compressed)
	if err != nil {
		return *httpext.NewResponse(ctx), errors.Wrap(err, "remote-write request failed")
	}
	res.Request.Body = ""

	return res, nil
}

func (c *Client) SetPreGeneratedLabelSets(ctx context.Context, labelSets []map[string]string) {
	c.preGeneratedLabelSets = make([][]prompb.Label, len(labelSets))
	for labelSetIdx := range labelSets {
		c.preGeneratedLabelSets[labelSetIdx] = labelMapToPromPb(labelSets[labelSetIdx])
	}
}

func labelMapToPromPb(labelsIn map[string]string) []prompb.Label {
	labelsOut := make([]prompb.Label, 0, len(labelsIn))
	for label, value := range labelsIn {
		labelsOut = append(labelsOut, prompb.Label{Name: label, Value: value})
	}
	return labelsOut
}

// lower takes two numbers and returns the lower one of the two
func lower(a, b int64) int64 {
	if a < b {
		return a
	}
	return b
}

func (c *Client) StorePreGenerated(ctx context.Context, minValue, maxValue float64, timestamp, batch_size, batch_id int64) (httpext.Response, error) {
	// Required for k6 metrics
	state := lib.GetState(ctx)
	if state == nil {
		return *httpext.NewResponse(ctx), errors.New("State is nil")
	}

	timeSeries := timeSeriesPool.Get().([]prompb.TimeSeries)
	if int64(len(timeSeries)) != batch_size {
		// Ensure that timeSeries have a fixed len and cap of batch_size
		timeSeries = make([]prompb.TimeSeries, batch_size)
	}

	batch_start := (batch_id * batch_size) % int64(len(c.preGeneratedLabelSets))
	for seriesIdx := range timeSeries {
		seriesIdxBatched := batch_start + int64(seriesIdx)
		if seriesIdxBatched >= int64(len(c.preGeneratedLabelSets)) {
			timeSeries = timeSeries[:seriesIdx+1]
			break
		}

		timeSeries[seriesIdx].Labels = c.preGeneratedLabelSets[seriesIdxBatched]

		if len(timeSeries[seriesIdx].Samples) != 1 {
			timeSeries[seriesIdx].Samples = make([]prompb.Sample, 1)
		}
		timeSeries[seriesIdx].Samples[0].Timestamp = timestamp
		timeSeries[seriesIdx].Samples[0].Value = minValue + (rand.Float64() * (maxValue - minValue))
	}

	data, err := proto.Marshal(&prompb.WriteRequest{Timeseries: timeSeries})
	if err != nil {
		return *httpext.NewResponse(ctx), errors.Wrap(err, "failed to marshal remote-write request")
	}

	// If len of timeSeries has been reduced then we set it back to the original len&cap
	timeSeriesPool.Put(timeSeries[:batch_size])

	res, err := c.send(ctx, state, snappy.Encode(nil, data))
	if err != nil {
		return *httpext.NewResponse(ctx), errors.Wrap(err, "remote-write request failed")
	}
	res.Request.Body = ""

	return res, nil
}

func (c *Client) StoreGenerated(ctx context.Context, total_series, batches, batch_size, batch int64) (httpext.Response, error) {
	ts, err := generate_series(total_series, batches, batch_size, batch)
	if err != nil {
		return *httpext.NewResponse(ctx), err
	}
	return c.Store(ctx, ts)
}

func generate_series(total_series, batches, batch_size, batch int64) ([]Timeseries, error) {
	if total_series == 0 {
		return nil, nil
	}
	if batch > batches {
		return nil, errors.New("batch must be in the range of batches")
	}
	if total_series/batches != batch_size {
		return nil, errors.New("total_series must divide evenly into batches of size batch_size")
	}

	series := make([]Timeseries, batch_size)
	timestamp := time.Now().UnixNano() / int64(time.Millisecond)
	for i := int64(0); i < batch_size; i++ {
		series_id := batch_size*(batch-1) + i
		labels := generate_cardinality_labels(total_series, series_id)
		labels = append(labels, Label{
			Name:  "__name__",
			Value: "k6_generated_metric_" + strconv.Itoa(int(series_id)),
		})

		// Required for querying in order to have unique series excluding the metric name.
		labels = append(labels, Label{
			Name:  "series_id",
			Value: strconv.Itoa(int(series_id)),
		})

		series[i] = Timeseries{
			labels,
			[]Sample{{rand.Float64() * 100, timestamp}},
		}
	}

	return series, nil
}

func generate_cardinality_labels(total_series, series_id int64) []Label {
	// exp is the greatest exponent of 10 that is less than total series.
	exp := int64(math.Log10(2000))
	labels := make([]Label, 0, exp)
	for x := 1; int64(x) <= exp; x++ {
		labels = append(labels, Label{
			Name:  "cardinality_1e" + strconv.Itoa(x),
			Value: strconv.Itoa(int(series_id / int64(math.Pow(10, float64(x))))),
		})
	}
	return labels
}

// send sends a batch of samples to the HTTP endpoint, the request is the proto marshalled
// and encoded bytes
func (c *Client) send(ctx context.Context, state *lib.State, req []byte) (httpext.Response, error) {
	httpResp := httpext.NewResponse(ctx)
	r, err := http.NewRequest("POST", c.cfg.Url, nil)
	if err != nil {
		return *httpResp, err
	}
	r.Header.Add("Content-Encoding", "snappy")
	r.Header.Set("Content-Type", "application/x-protobuf")
	r.Header.Set("User-Agent", c.cfg.UserAgent)
	r.Header.Set("X-Prometheus-Remote-Write-Version", "0.0.2")
	if c.cfg.TenantName != "" {
		r.Header.Set("X-Scope-OrgID", c.cfg.TenantName)
	}

	duration, err := str2duration.ParseDuration(c.cfg.Timeout)
	if err != nil {
		return *httpResp, err
	}

	u, err := url.Parse(c.cfg.Url)
	if err != nil {
		return *httpResp, err
	}

	url, _ := httpext.NewURL(c.cfg.Url, u.Host+u.Path)
	response, err := httpext.MakeRequest(ctx, &httpext.ParsedHTTPRequest{
		URL:              &url,
		Req:              r,
		Body:             bytes.NewBuffer(req),
		Throw:            state.Options.Throw.Bool,
		Redirects:        state.Options.MaxRedirects,
		Timeout:          duration,
		ResponseCallback: ResponseCallback,
	})
	if err != nil {
		return *httpResp, err
	}

	return *response, err
}

func ResponseCallback(n int) bool {
	return n == 200
}

func FromTimeseriesToPrometheusTimeseries(ts Timeseries) prompb.TimeSeries {
	var labels []prompb.Label
	var samples []prompb.Sample
	for _, label := range ts.Labels {
		labels = append(labels, prompb.Label{
			Name:  label.Name,
			Value: label.Value,
		})
	}
	for _, sample := range ts.Samples {
		if sample.Timestamp == 0 {
			sample.Timestamp = time.Now().UnixNano() / int64(time.Millisecond)
		}
		samples = append(samples, prompb.Sample{
			Value:     sample.Value,
			Timestamp: sample.Timestamp,
		})
	}

	return prompb.TimeSeries{
		Labels:  labels,
		Samples: samples,
	}
}
