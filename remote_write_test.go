package remotewrite

import (
	"fmt"
	"math/rand"
	"reflect"
	"testing"
	"time"

	"github.com/prometheus/prometheus/prompb"
	"github.com/stretchr/testify/require"
)

func TestEvaluateTemplate(t *testing.T) {
	testcases := []struct {
		template      string
		value         int
		result        string
		expectedError string
	}{
		{template: "something ${series_id} else", value: 12, result: "something 12 else"},
		{template: "something ${series_id else", expectedError: "unsupported template"},
		{template: "something ${series_id/6} else", value: 12, result: "something 2 else"},
		{template: "something ${series_id/6 else", expectedError: "closing bracket"},
		{template: "something ${series_id%6} else", value: 12, result: "something 0 else"},
		{template: "something ${series_id%6 else", expectedError: "closing bracket"},
		{template: "something ${series_id*6} else", expectedError: "unsupported template"},
		{template: "something else", result: "something else"},
	}
	for _, testcase := range testcases {
		testcase := testcase
		t.Run(fmt.Sprintf("template=%q,value=%d", testcase.template, testcase.value), func(t *testing.T) {
			compiled, err := compileTemplate(testcase.template)
			if testcase.expectedError != "" {
				require.Error(t, err)
				require.Contains(t, err.Error(), testcase.expectedError)
				return
			}
			require.NoError(t, err)
			result := compiled.ToString(testcase.value)
			require.Equal(t, testcase.result, result)
		})
	}
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
					"series_id":       "${series_id}",
					"cardinality_1e1": "${series_id/10}",
					"cardinality_1e3": "${series_id/1000}",
					"cardinality_2":   "${series_id%2}",
					"cardinality_10":  "${series_id%10}",
				},
			},
			want: want{
				valueMin: 123,
				valueMax: 133,
				series: []prompb.TimeSeries{
					{
						Labels: []prompb.Label{
							{Name: "__name__", Value: "k6_generated_metric_50"},
							{Name: "cardinality_10", Value: "0"},
							{Name: "cardinality_1e1", Value: "5"},
							{Name: "cardinality_1e3", Value: "0"},
							{Name: "cardinality_2", Value: "0"},
							{Name: "series_id", Value: "50"},
						},
						Samples: []prompb.Sample{{Timestamp: 123456789}},
					}, {
						Labels: []prompb.Label{
							{Name: "__name__", Value: "k6_generated_metric_51"},
							{Name: "cardinality_10", Value: "1"},
							{Name: "cardinality_1e1", Value: "5"},
							{Name: "cardinality_1e3", Value: "0"},
							{Name: "cardinality_2", Value: "1"},
							{Name: "series_id", Value: "51"},
						},
						Samples: []prompb.Sample{{Timestamp: 123456789}},
					}, {
						Labels: []prompb.Label{
							{Name: "__name__", Value: "k6_generated_metric_52"},
							{Name: "cardinality_10", Value: "2"},
							{Name: "cardinality_1e1", Value: "5"},
							{Name: "cardinality_1e3", Value: "0"},
							{Name: "cardinality_2", Value: "0"},
							{Name: "series_id", Value: "52"},
						},
						Samples: []prompb.Sample{{Timestamp: 123456789}},
					}, {
						Labels: []prompb.Label{
							{Name: "__name__", Value: "k6_generated_metric_53"},
							{Name: "cardinality_10", Value: "3"},
							{Name: "cardinality_1e1", Value: "5"},
							{Name: "cardinality_1e3", Value: "0"},
							{Name: "cardinality_2", Value: "1"},
							{Name: "series_id", Value: "53"},
						},
						Samples: []prompb.Sample{{Timestamp: 123456789}},
					}, {
						Labels: []prompb.Label{
							{Name: "__name__", Value: "k6_generated_metric_54"},
							{Name: "cardinality_10", Value: "4"},
							{Name: "cardinality_1e1", Value: "5"},
							{Name: "cardinality_1e3", Value: "0"},
							{Name: "cardinality_2", Value: "0"},
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
			r := rand.New(rand.NewSource(time.Now().Unix()))
			got, err := generateFromTemplates(r, tt.args.minValue, tt.args.maxValue, tt.args.timestamp, tt.args.minSeriesID, tt.args.maxSeriesID, tt.args.labelsTemplate)
			require.NoError(t, err)
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
