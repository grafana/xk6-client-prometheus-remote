package remotewrite

import "testing"

func BenchmarkEvaluateTemplatesSimple(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_ = evaluateTemplate("something ${series_id} else", 1151234)
	}
}

func BenchmarkEvaluateTemplatesComplex(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_ = evaluateTemplate("something ${series_id/1000} else", 1151234)
	}
}
