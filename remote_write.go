package remotewrite

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"net/http"
	"net/url"
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

// Register the extension on module initialization, available to
// import from JS as "k6/x/remotewrite".
func init() {
	modules.Register("k6/x/remotewrite", new(RemoteWrite))
}

// RemoteWrite is the k6 extension for interacting with Kubernetes jobs.
type RemoteWrite struct {
}

// Client is the client wrapper.
type Client struct {
	client *http.Client
	cfg    *Config
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
		config.UserAgent = "k6-remote-write/0.0.1"
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
	Labels []struct {
		Name  string `json:"name"`
		Value string `json:"value"`
	} `json:"labels"`
	Samples []struct {
		Value     float64 `json:"value"`
		Timestamp int64   `json:"timestamp"`
	} `json:"samples"`
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

	return res, nil
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
	r.Header.Set("X-Prometheus-Remote-Write-Version", "0.1.0")
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
		URL:       &url,
		Req:       r,
		Body:      bytes.NewBuffer(req),
		Throw:     state.Options.Throw.Bool,
		Redirects: state.Options.MaxRedirects,
		Timeout:   duration,
	})
	if err != nil {
		return *httpResp, err
	}

	return *response, err
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
