package remotewrite

import "go.k6.io/k6/stats"

var (
	// PrometheusRemoteWrite-related.
	RemoteWriteReqs        = stats.New("remote_write_reqs", stats.Counter)
	RemoteWriteReqFailed   = stats.New("remote_write_req_failed", stats.Rate)
	RemoteWriteReqDuration = stats.New("remote_write_req_duration", stats.Trend, stats.Time)
	RemoteWriteNumSeries   = stats.New("remote_write_num_series", stats.Counter)
)
