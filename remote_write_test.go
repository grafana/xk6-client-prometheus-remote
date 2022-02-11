package remotewrite

import (
	"reflect"
	"testing"

	"github.com/prometheus/prometheus/prompb"
	"github.com/stretchr/testify/require"
)

func TestEvaluateTemplate(t *testing.T) {
	require.Equal(t, compileTemplate("something ${series_id} else")(12), "something 12 else")
	require.Equal(t, compileTemplate("something ${series_id/6} else")(12), "something 2 else")
}

func TestGenerateFromTemplates(t *testing.T) {
	type args struct {
		minValue       int
		maxValue       int
		timestamp      int64
		minSeriesID    int
		maxSeriesID    int
		labelsTemplate map[string]string
	}
	type want struct {
		valueMin float64
		valueMax float64
		series   []prompb.TimeSeries
	}
	tests := []struct {
		name string
		args args
		want want
	}{
		{
			name: "11th batch of 5",
			args: args{
				minValue:    123,
				maxValue:    133,
				timestamp:   123456789,
				minSeriesID: 50,
				maxSeriesID: 55,
				labelsTemplate: map[string]string{
					"__name__":        "k6_generated_metric_${series_id}",
					"cardinality_1e1": "${series_id/10}",
					"cardinality_1e3": "${series_id/1000}",
					"series_id":       "${series_id}",
				},
			},
			want: want{
				valueMin: 123,
				valueMax: 133,
				series: []prompb.TimeSeries{
					{
						Labels: []prompb.Label{
							{Name: "__name__", Value: "k6_generated_metric_50"},
							{Name: "cardinality_1e1", Value: "5"},
							{Name: "cardinality_1e3", Value: "0"},
							{Name: "series_id", Value: "50"},
						},
						Samples: []prompb.Sample{{Timestamp: 123456789}},
					}, {
						Labels: []prompb.Label{
							{Name: "__name__", Value: "k6_generated_metric_51"},
							{Name: "cardinality_1e1", Value: "5"},
							{Name: "cardinality_1e3", Value: "0"},
							{Name: "series_id", Value: "51"},
						},
						Samples: []prompb.Sample{{Timestamp: 123456789}},
					}, {
						Labels: []prompb.Label{
							{Name: "__name__", Value: "k6_generated_metric_52"},
							{Name: "cardinality_1e1", Value: "5"},
							{Name: "cardinality_1e3", Value: "0"},
							{Name: "series_id", Value: "52"},
						},
						Samples: []prompb.Sample{{Timestamp: 123456789}},
					}, {
						Labels: []prompb.Label{
							{Name: "__name__", Value: "k6_generated_metric_53"},
							{Name: "cardinality_1e1", Value: "5"},
							{Name: "cardinality_1e3", Value: "0"},
							{Name: "series_id", Value: "53"},
						},
						Samples: []prompb.Sample{{Timestamp: 123456789}},
					}, {
						Labels: []prompb.Label{
							{Name: "__name__", Value: "k6_generated_metric_54"},
							{Name: "cardinality_1e1", Value: "5"},
							{Name: "cardinality_1e3", Value: "0"},
							{Name: "series_id", Value: "54"},
						},
						Samples: []prompb.Sample{{Timestamp: 123456789}},
					},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := generateFromTemplates(tt.args.minValue, tt.args.maxValue, tt.args.timestamp, tt.args.minSeriesID, tt.args.maxSeriesID, tt.args.labelsTemplate)
			if len(got) != len(tt.want.series) {
				t.Errorf("Differing length, want: %d, got: %d", len(tt.want.series), len(got))
			}

			for seriesId := range got {
				if !reflect.DeepEqual(got[seriesId].Labels, tt.want.series[seriesId].Labels) {
					t.Errorf("Unexpected labels in series %d, want: %v, got: %v", seriesId, tt.want.series[seriesId].Labels, got[seriesId].Labels)
				}

				if got[seriesId].Samples[0].Timestamp != tt.want.series[seriesId].Samples[0].Timestamp {
					t.Errorf("Unexpected timestamp in series %d, want: %d, got: %d", seriesId, tt.want.series[seriesId].Samples[0].Timestamp, got[seriesId].Samples[0].Timestamp)
				}

				if got[seriesId].Samples[0].Value < tt.want.valueMin || got[seriesId].Samples[0].Value > tt.want.valueMax {
					t.Errorf("Unexpected value in series %d, want: %f-%f, got: %f", seriesId, tt.want.valueMin, tt.want.valueMax, got[seriesId].Samples[0].Value)
				}
			}
		})
	}
}
