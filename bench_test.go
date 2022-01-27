package remotewrite

import "testing"

func BenchmarkCompileTemplatesSimple(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_ = compileTemplate("something ${series_id} else")
	}
}

func BenchmarkCompileTemplatesComplex(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_ = compileTemplate("something ${series_id/1000} else")
	}
}

func BenchmarkEvaluateTemplatesSimple(b *testing.B) {
	t := compileTemplate("something ${series_id} else")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = t(1151234)
	}
}

func BenchmarkEvaluateTemplatesComplex(b *testing.B) {
	t := compileTemplate("something ${series_id/1000} else")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = t(1151234)
	}
}
