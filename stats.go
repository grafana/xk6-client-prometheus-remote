package remotewrite

import "github.com/loadimpact/k6/stats"

var (
	RequestsTotal = stats.New("remote_write_req_total", stats.Counter)

	RequestsDuration = stats.New("remote_write_req_duration", stats.Trend, stats.Time)
)
