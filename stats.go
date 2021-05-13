package remotewrite

import "go.k6.io/k6/stats"

var (
	DataSent = stats.New("data_sent", stats.Counter, stats.Data)

	RequestsTotal    = stats.New("remote_write_req_total", stats.Counter)
	RequestsDuration = stats.New("remote_write_req_duration", stats.Trend, stats.Time)
)
