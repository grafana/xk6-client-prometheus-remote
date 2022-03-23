package remotewrite

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"log"
	"math"
	"math/rand"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/dop251/goja"
	"github.com/golang/protobuf/proto"
	"github.com/golang/snappy"
	"github.com/pkg/errors"
	"github.com/prometheus/prometheus/prompb"
	"github.com/xhit/go-str2duration/v2"
	"go.k6.io/k6/js/common"
	"go.k6.io/k6/js/modules"
	"go.k6.io/k6/lib"
	"go.k6.io/k6/lib/netext/httpext"
	"google.golang.org/protobuf/encoding/protowire"
)

// Register the extension on module initialization, available to
// import from JS as "k6/x/remotewrite".
func init() {
	modules.Register("k6/x/remotewrite", new(remoteWriteModule))
}

// RemoteWrite is the k6 extension for interacting Prometheus Remote Write endpoints.
type RemoteWrite struct {
	vu modules.VU
}

type remoteWriteModule struct{}

var _ modules.Module = &remoteWriteModule{}

func (r *remoteWriteModule) NewModuleInstance(vu modules.VU) modules.Instance {
	return &RemoteWrite{
		vu: vu,
	}
}

func (r *RemoteWrite) Exports() modules.Exports {
	return modules.Exports{
		Named: map[string]interface{}{
			"Client":                   r.xclient,
			"Sample":                   r.sample,
			"Timeseries":               r.timeseries,
			"precompileLabelTemplates": compileLabelTemplates,
		},
	}
}

// Client is the client wrapper.
type Client struct {
	client *http.Client
	cfg    *Config
	vu     modules.VU
}

type Config struct {
	Url        string `json:"url"`
	UserAgent  string `json:"user_agent"`
	Timeout    string `json:"timeout"`
	TenantName string `json:"tenant_name"`
}

// xclient represents
func (r *RemoteWrite) xclient(c goja.ConstructorCall) *goja.Object {
	var config Config
	rt := r.vu.Runtime()
	err := rt.ExportTo(c.Argument(0), &config)
	if err != nil {
		common.Throw(rt, fmt.Errorf("Client constructor expects first argument to be Config"))
	}
	if config.Url == "" {
		log.Fatal(fmt.Errorf("url is required"))
	}
	if config.UserAgent == "" {
		config.UserAgent = "k6-remote-write/0.0.2"
	}
	if config.Timeout == "" {
		config.Timeout = "10s"
	}

	return rt.ToValue(&Client{
		client: &http.Client{},
		cfg:    &config,
		vu:     r.vu,
	}).ToObject(rt)
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

func (r *RemoteWrite) sample(c goja.ConstructorCall) *goja.Object {
	rt := r.vu.Runtime()
	call, _ := goja.AssertFunction(rt.ToValue(xsample))
	v, err := call(goja.Undefined(), c.Arguments...)
	if err != nil {
		common.Throw(rt, err)
	}
	return v.ToObject(rt)
}

func xsample(value float64, timestamp int64) Sample {
	return Sample{
		Value:     value,
		Timestamp: timestamp,
	}
}

func (r *RemoteWrite) timeseries(c goja.ConstructorCall) *goja.Object {
	rt := r.vu.Runtime()
	call, _ := goja.AssertFunction(rt.ToValue(xtimeseries))
	v, err := call(goja.Undefined(), c.Arguments...)
	if err != nil {
		common.Throw(rt, err)
	}
	return v.ToObject(rt)
}

func xtimeseries(labels map[string]string, samples []Sample) *Timeseries {
	t := &Timeseries{
		Labels:  make([]Label, 0, len(labels)),
		Samples: samples,
	}

	for k, v := range labels {
		t.Labels = append(t.Labels, Label{Name: k, Value: v})
	}

	return t
}

func (c *Client) StoreGenerated(total_series, batches, batch_size, batch int64) (httpext.Response, error) {
	ts, err := generate_series(total_series, batches, batch_size, batch)
	if err != nil {
		return *httpext.NewResponse(), err
	}
	return c.Store(ts)
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

	r := rand.New(rand.NewSource(time.Now().Unix()))
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
			[]Sample{{r.Float64() * 100, timestamp}},
		}
	}

	return series, nil
}

func generate_cardinality_labels(total_series, series_id int64) []Label {
	// exp is the greatest exponent of 10 that is less than total series.
	exp := int64(math.Log10(float64(total_series)))
	labels := make([]Label, 0, exp)
	for x := 1; int64(x) <= exp; x++ {
		labels = append(labels, Label{
			Name:  "cardinality_1e" + strconv.Itoa(x),
			Value: strconv.Itoa(int(series_id / int64(math.Pow(10, float64(x))))),
		})
	}
	return labels
}

func (c *Client) Store(ts []Timeseries) (httpext.Response, error) {
	var batch []prompb.TimeSeries
	for _, t := range ts {
		batch = append(batch, FromTimeseriesToPrometheusTimeseries(t))
	}
	return c.store(batch)
}

func (c *Client) store(batch []prompb.TimeSeries) (httpext.Response, error) {
	// Required for k6 metrics
	state := c.vu.State()
	if state == nil {
		return *httpext.NewResponse(), errors.New("State is nil")
	}

	req := prompb.WriteRequest{
		Timeseries: batch,
	}

	data, err := proto.Marshal(&req)
	if err != nil {
		return *httpext.NewResponse(), errors.Wrap(err, "failed to marshal remote-write request")
	}

	compressed := snappy.Encode(nil, data)

	res, err := c.send(state, compressed)
	if err != nil {
		return *httpext.NewResponse(), errors.Wrap(err, "remote-write request failed")
	}
	res.Request.Body = ""

	return res, nil
}

// send sends a batch of samples to the HTTP endpoint, the request is the proto marshalled
// and encoded bytes
func (c *Client) send(state *lib.State, req []byte) (httpext.Response, error) {
	httpResp := httpext.NewResponse()
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
	response, err := httpext.MakeRequest(c.vu.Context(), state, &httpext.ParsedHTTPRequest{
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

// The only supported things are:
// 1. replacing ${series_id} with the series_id provided
// 2. replacing ${series_id/<integer>} with the evaluation of that
// 3. if error in parsing return error
func compileTemplate(template string) (*labelGenerator, error) {
	i := strings.Index(template, "${series_id")
	if i == -1 {
		return newIdentityLabelGenerator(template), nil
	}
	switch template[i+len("${series_id")] {
	case '}':
		return &labelGenerator{
			ToString: func(seriesID int) string {
				return template[:i] + strconv.Itoa(seriesID) + template[i+len("${series_id}"):]
			},
			AppendByte: func(b []byte, seriesID int) []byte {
				b = append(b, template[:i]...)
				b = strconv.AppendInt(b, int64(seriesID), 10)
				return append(b, template[i+len("${series_id}"):]...)
			},
		}, nil
	case '%':
		end := strings.Index(template[i:], "}")
		if end == -1 {
			return nil, errors.New("no closing bracket in template")
		}
		d, err := strconv.Atoi(template[i+len("${series_id%") : i+end])
		if err != nil {
			return nil, fmt.Errorf("can't parse divisor of the module operator %w", err)
		}

		possibleValues := make([][]byte, d)
		// TODO have an upper limit
		for j := 0; j < d; j++ {
			var b []byte
			b = append(b, template[:i]...)
			b = strconv.AppendInt(b, int64(j), 10)
			possibleValues[j] = append(b, template[i+end+1:]...)
		}
		possibleValuesS := make([]string, d)
		// TODO have an upper limit
		for j := 0; j < d; j++ {
			possibleValuesS[j] = template[:i] + strconv.Itoa((j)) + template[i+end+1:]
		}
		return &labelGenerator{
			ToString: func(seriesID int) string {
				return possibleValuesS[seriesID%d]
			},
			AppendByte: func(b []byte, seriesID int) []byte {
				return append(b, possibleValues[seriesID%d]...)
			},
		}, nil
	case '/':
		end := strings.Index(template[i:], "}")
		if end == -1 {
			return nil, errors.New("no closing bracket in template")
		}
		d, err := strconv.Atoi(template[i+len("${series_id/") : i+end])
		if err != nil {
			return nil, err
		}
		var memoizeS string
		var memoizeSValue int

		var memoize []byte
		var memoizeValue int64
		return &labelGenerator{
			ToString: func(seriesID int) string {
				value := (seriesID / d)
				if memoizeS == "" || value != memoizeSValue {
					memoizeSValue = value
					memoizeS = template[:i] + strconv.Itoa(value) + template[i+end+1:]
				}
				return memoizeS
			},
			AppendByte: func(b []byte, seriesID int) []byte {
				value := int64(seriesID / d)
				if memoize == nil || value != memoizeValue {
					memoizeValue = value
					memoize = memoize[:0]
					memoize = append(memoize, template[:i]...)
					memoize = strconv.AppendInt(memoize, value, 10)
					memoize = append(memoize, template[i+end+1:]...)
				}
				return append(b, memoize...)
			},
		}, nil
	}
	return nil, errors.New("unsupported template")
}

type labelGenerator struct {
	ToString   func(int) string
	AppendByte func([]byte, int) []byte
}

func newIdentityLabelGenerator(t string) *labelGenerator {
	return &labelGenerator{
		ToString:   func(int) string { return t },
		AppendByte: func(b []byte, _ int) []byte { return append(b, t...) },
	}
}

func generateFromTemplates(r *rand.Rand, minValue, maxValue int,
	timestamp int64, minSeriesID, maxSeriesID int,
	labelsTemplate map[string]string,
) ([]prompb.TimeSeries, error) {
	batchSize := maxSeriesID - minSeriesID
	series := make([]prompb.TimeSeries, batchSize)

	compiledTemplates, err := compileLabelTemplates(labelsTemplate)
	if err != nil {
		return nil, err
	}
	for seriesID := minSeriesID; seriesID < maxSeriesID; seriesID++ {
		labels := make([]prompb.Label, len(labelsTemplate))
		// TODO optimize
		for i, template := range compiledTemplates.compiledTemplates {
			labels[i] = prompb.Label{Name: template.name, Value: template.generator.ToString(seriesID)}
		}

		series[seriesID-minSeriesID] = prompb.TimeSeries{
			Labels: labels,
			Samples: []prompb.Sample{
				{
					Value:     valueBetween(r, minValue, maxValue),
					Timestamp: timestamp,
				},
			},
		}
	}

	return series, nil
}

// this is opaque on purpose so that it can't be done anything to from the js side
type labelTemplates struct {
	compiledTemplates []compiledTemplate
	labelValue        []byte
}
type compiledTemplate struct {
	name      string
	generator *labelGenerator
}

func compileLabelTemplates(labelsTemplate map[string]string) (*labelTemplates, error) {
	compiledTemplates := make([]compiledTemplate, len(labelsTemplate))
	{
		i := 0
		var err error
		for k, v := range labelsTemplate {
			compiledTemplates[i].name = k
			compiledTemplates[i].generator, err = compileTemplate(v)
			if err != nil {
				return nil, fmt.Errorf("error while compiling template %q, %w", v, err)
			}
			i++
		}
	}
	sort.Slice(compiledTemplates, func(i, j int) bool {
		return compiledTemplates[i].name < compiledTemplates[j].name
	})
	return &labelTemplates{
		compiledTemplates: compiledTemplates,
		labelValue:        make([]byte, 128), // this is way more than necessary and it will grow if needed
	}, nil
}

func (c *Client) StoreFromTemplates(
	minValue, maxValue int,
	timestamp int64, minSeriesID, maxSeriesID int,
	labelsTemplate map[string]string,
) (httpext.Response, error) {
	r := rand.New(rand.NewSource(time.Now().Unix()))
	ts, err := generateFromTemplates(r, minValue, maxValue, timestamp, minSeriesID, maxSeriesID, labelsTemplate)
	if err != nil {
		return httpext.Response{}, err
	}
	return c.store(ts)
}

func (template *labelTemplates) writeFor(w *bytes.Buffer, value float64, seriesID int, timestamp int64) (err error) {
	labelValue := template.labelValue[:]
	for _, template := range template.compiledTemplates {
		labelValue = labelValue[:0]
		w.WriteByte(0xa)
		labelValue = protowire.AppendVarint(labelValue, uint64(len(template.name)))
		n1 := len(labelValue)
		labelValue = template.generator.AppendByte(labelValue, seriesID)
		n2 := len(labelValue)
		labelValue = protowire.AppendVarint(labelValue, uint64(n2-n1))
		n3 := len(labelValue)

		labelValue = protowire.AppendVarint(labelValue, uint64(n3+1+1+len(template.name)))
		w.Write(labelValue[n3:])
		w.WriteByte(0xa)
		w.Write(labelValue[:n1])
		w.WriteString(template.name)
		w.WriteByte(0x12)
		w.Write(labelValue[n2:n3])
		w.Write(labelValue[n1:n2])
	}

	labelValue = labelValue[:10]
	labelValue[0] = 0x9
	binary.LittleEndian.PutUint64(labelValue[1:9], uint64(math.Float64bits(value)))
	labelValue[9] = 0x10
	labelValue = protowire.AppendVarint(labelValue, uint64(timestamp))

	n := len(labelValue)
	labelValue = labelValue[:n+1]
	labelValue[n] = 0x12
	labelValue = protowire.AppendVarint(labelValue, uint64(n))
	w.Write(labelValue[n:])
	w.Write(labelValue[:n])
	template.labelValue = labelValue
	return nil // TODO fix
}

func (c *Client) StoreFromPrecompiledTemplates(
	minValue, maxValue int,
	timestamp int64, minSeriesID, maxSeriesID int,
	template *labelTemplates,
) (httpext.Response, error) {
	state := c.vu.State()
	if state == nil {
		return *httpext.NewResponse(), errors.New("State is nil")
	}
	r := rand.New(rand.NewSource(time.Now().Unix()))
	buf := generateFromPrecompiledTemplates(r, minValue, maxValue, timestamp, minSeriesID, maxSeriesID, template)
	b := buf.Bytes()
	compressed := make([]byte, len(b)/9) // the general size is actually between 1/9 and 1/10th but this is closed enough
	compressed = snappy.Encode(compressed, b)

	res, err := c.send(state, compressed)
	if err != nil {
		return *httpext.NewResponse(), errors.Wrap(err, "remote-write request failed")
	}
	res.Request.Body = ""

	return res, nil
}

func generateFromPrecompiledTemplates(
	r *rand.Rand,
	minValue, maxValue int,
	timestamp int64, minSeriesID, maxSeriesID int,
	template *labelTemplates,
) *bytes.Buffer {
	bigB := make([]byte, 1024)
	buf := new(bytes.Buffer)
	buf.Reset()

	tsBuf := new(bytes.Buffer)
	bigB[0] = 0xa
	template.writeFor(tsBuf, valueBetween(r, minValue, maxValue), minSeriesID, timestamp)
	bigB = protowire.AppendVarint(bigB[:1], uint64(tsBuf.Len()))
	buf.Write(bigB)
	tsBuf.WriteTo(buf)

	buf.Grow((buf.Len() + 2) * (maxSeriesID - minSeriesID)) // heuristics to try to get big enough buffer in one go
	for seriesID := minSeriesID + 1; seriesID < maxSeriesID; seriesID++ {
		tsBuf.Reset()
		bigB[0] = 0xa
		template.writeFor(tsBuf, valueBetween(r, minValue, maxValue), seriesID, timestamp)
		bigB = protowire.AppendVarint(bigB[:1], uint64(tsBuf.Len()))
		buf.Write(bigB)
		tsBuf.WriteTo(buf)
	}

	return buf
}

func valueBetween(r *rand.Rand, min, max int) float64 {
	return (r.Float64() * float64(max-min)) + float64(min)
}
