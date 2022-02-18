package remotewrite

import (
	"io"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"testing"

	"go.k6.io/k6/js/modulestest"
	"go.k6.io/k6/lib"
)

func BenchmarkCompileTemplatesSimple(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_, _ = compileTemplate("something ${series_id} else")
	}
}

func BenchmarkCompileTemplatesComplex(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_, _ = compileTemplate("something ${series_id/1000} else")
	}
}

func BenchmarkEvaluateTemplatesSimple(b *testing.B) {
	t, _ := compileTemplate("something ${series_id} else")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = t(1151234)
	}
}

func BenchmarkEvaluateTemplatesComplex(b *testing.B) {
	t, _ := compileTemplate("something ${series_id/1000} else")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = t(1151234)
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

func BenchmarkStoreFromPrecompiledTemplates(b *testing.B) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.Copy(ioutil.Discard, r.Body)
		w.WriteHeader(200)
	}))
	b.Cleanup(func() {
		s.Close()
	})
	vu := new(modulestest.VU)
	vu.StateField = new(lib.State)
	c := &Client{
		client: &http.Client{},
		cfg: &Config{
			Url: s.URL,
		},
		vu: vu,
	}
	template := precompileLabelTemplates(benchmarkLabels)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		c.StoreFromPrecompiledTemplates(i, i+10, int64(i), 0, 100000, template)
	}
}

func BenchmarkStoreFromTemplates(b *testing.B) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.Copy(ioutil.Discard, r.Body)
		w.WriteHeader(200)
	}))
	b.Cleanup(func() {
		s.Close()
	})
	vu := new(modulestest.VU)
	vu.StateField = new(lib.State)
	c := &Client{
		client: &http.Client{},
		cfg: &Config{
			Url: s.URL,
		},
		vu: vu,
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		c.StoreFromTemplates(i, i+10, int64(i), 0, 100000, benchmarkLabels)
	}
}
