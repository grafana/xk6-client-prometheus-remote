package remotewrite

import (
	"context"
	"io"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/golang/protobuf/proto"
	"github.com/oxtoacart/bpool"
	"github.com/prometheus/prometheus/prompb"
	"github.com/stretchr/testify/require"
	"go.k6.io/k6/js/modulestest"
	"go.k6.io/k6/lib"
	"go.k6.io/k6/lib/metrics"
	"go.k6.io/k6/stats"
)

func BenchmarkCompileTemplatesSimple(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_, err := compileTemplate("something ${series_id} else")
		require.NoError(b, err)
	}
}

func BenchmarkCompileTemplatesComplex(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_, err := compileTemplate("something ${series_id/1000} else")
		require.NoError(b, err)
	}
}

func BenchmarkEvaluateTemplatesSimple(b *testing.B) {
	t, err := compileTemplate("something ${series_id} else")
	require.NoError(b, err)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = t.ToString(i)
	}
}

func BenchmarkEvaluateTemplatesComplex(b *testing.B) {
	t, err := compileTemplate("something ${series_id/1000} else")
	require.NoError(b, err)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = t.ToString(i)
	}
}

var benchmarkLabels = map[string]string{
	"__name__":        "k6_generated_metric_${series_id/1000}",
	"series_id":       "${series_id}",
	"cardinality_1e1": "${series_id/10}",
	"cardinality_1e2": "${series_id/100}",
	"cardinality_1e3": "${series_id/1000}",
	"cardinality_1e4": "${series_id/10000}",
	"cardinality_1e5": "${series_id/100000}",
	"cardinality_1e6": "${series_id/1000000}",
	"cardinality_1e7": "${series_id/10000000}",
	"cardinality_2":   "${series_id%2}",
	"cardinality_50":  "${series_id%50}",
}

type testServer struct {
	server *httptest.Server
	vu     *modulestest.VU
	count  *int64
}

func newTestServer(tb testing.TB) *testServer {
	ts := &testServer{
		count: new(int64),
	}

	ts.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.Copy(io.Discard, r.Body)
		w.WriteHeader(200)
		atomic.AddInt64(ts.count, 1)
	}))
	ch := make(chan stats.SampleContainer)
	tb.Cleanup(func() {
		ts.server.Close()
		close(ch) // this might need to be elsewhere
	})
	ts.vu = new(modulestest.VU)
	ts.vu.StateField = new(lib.State)
	ts.vu.CtxField = context.Background()
	ts.vu.StateField.Tags = lib.NewTagMap(nil)
	ts.vu.StateField.Transport = ts.server.Client().Transport
	ts.vu.StateField.BPool = bpool.NewBufferPool(123)
	ts.vu.StateField.Samples = ch
	ts.vu.StateField.BuiltinMetrics = metrics.RegisterBuiltinMetrics(metrics.NewRegistry())

	go func() {
		for range ch {
		}
	}()
	return ts
}

func BenchmarkStoreFromTemplates(b *testing.B) {
	s := newTestServer(b)
	c := &Client{
		client: &http.Client{},
		cfg: &Config{
			Url:     s.server.URL,
			Timeout: "100s",
		},
		vu: s.vu,
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := c.StoreFromTemplates(i, i+10, int64(i), 0, 100000, benchmarkLabels)
		require.NoError(b, err)
	}
	require.True(b, 1 <= *s.count) // this might need an atomic
}

func BenchmarkGenerateFromTemplates(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		r := rand.New(rand.NewSource(time.Now().Unix()))
		for pb.Next() {
			i++
			_, err := generateFromTemplates(r, i, i+10, int64(i), 0, 100000, benchmarkLabels)
			require.NoError(b, err)
		}
	})
}

func BenchmarkGenerateFromTemplatesAndMarshal(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		r := rand.New(rand.NewSource(time.Now().Unix()))
		for pb.Next() {
			i++
			batch, err := generateFromTemplates(r, i, i+10, int64(i), 0, 100000, benchmarkLabels)
			require.NoError(b, err)

			req := prompb.WriteRequest{
				Timeseries: batch,
			}
			_, err = proto.Marshal(&req)
			require.NoError(b, err)
		}
	})
}
