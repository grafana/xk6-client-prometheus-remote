# xk6-client-prometheus-remote

This extension adds Prometheus Remote Write testing capabilities to [k6](https://go.k6.io/k6). You can test any service that accepts data via Prometheus remote_write API such as [Cortex](https://github.com/cortexproject/cortex), [Thanos](https://github.com/improbable-eng/thanos), [Prometheus itself](https://prometheus.io/docs/prometheus/latest/feature_flags/#remote-write-receiver) and other services [listed here](https://prometheus.io/docs/operating/integrations/#remote-endpoints-and-storage).

It is implemented using the [xk6](https://k6.io/blog/extending-k6-with-xk6/) system.

> :warning: Not to be confused with [Prometheus remote write **output** extension](https://github.com/grafana/xk6-output-prometheus-remote) which is publishing test-run metrics to Prometheus.

## Getting started  

To start using k6 with the extension you can:
- Download and run the [binaries](https://github.com/grafana/xk6-client-prometheus-remote/releases) that we build on each release.
- Build your own binary from the source.

If you wanna go with the last option, first, ensure you have the prerequisites:

- [Go toolchain](https://go101.org/article/go-toolchain.html)
- Git

Then:

1. Install `xk6`:
  ```shell
  $ go install go.k6.io/xk6/cmd/xk6@latest
  ```

2. Build the binary:
  ```shell
  $ xk6 build --with github.com/grafana/xk6-client-prometheus-remote@latest
  ```

## Basic Example

```javascript
import { check, sleep } from 'k6';
import remote from 'k6/x/remotewrite';

export let options = {
    vus: 10,
    duration: '10s',
};

const client = new remote.Client({
    url: "<your-remote-write-url>"
});

export default function () {
    let res = client.store([{
        "labels": [
            { "name": "__name__", "value": `my_cool_metric_${__VU}` },
            { "name": "service", "value": "bar" }
        ],
        "samples": [
            { "value": Math.random() * 100, }
        ]
    }]);
    check(res, {
        'is status 200': (r) => r.status === 200,
    });
    sleep(1)
}
```

Result output:

```
$ ./k6 run examples/basic.js

          /\      |‾‾| /‾‾/   /‾‾/   
     /\  /  \     |  |/  /   /  /    
    /  \/    \    |     (   /   ‾‾\  
   /          \   |  |\  \ |  (‾)  | 
  / __________ \  |__| \__\ \_____/ .io

  execution: local
     script: examples/basic.js
     output: -

  scenarios: (100.00%) 1 scenario, 10 max VUs, 40s max duration (incl. graceful stop):
           * default: 10 looping VUs for 10s (gracefulStop: 30s)


running (10.4s), 00/10 VUs, 90 complete and 0 interrupted iterations
default ✓ [======================================] 10 VUs  10s

     ✓ is status 200

     checks.....................: 100.00% ✓ 90       ✗ 0   
     data_received..............: 46 kB   4.4 kB/s
     data_sent..................: 24 kB   2.3 kB/s
     http_req_blocked...........: avg=7.52ms   min=290ns    med=380ns    max=68.08ms  p(90)=67.2ms   p(95)=67.7ms  
     http_req_connecting........: avg=1.88ms   min=0s       med=0s       max=18.27ms  p(90)=15.25ms  p(95)=17.11ms 
     http_req_duration..........: avg=136.88ms min=131.24ms med=135.66ms max=215.49ms p(90)=139.2ms  p(95)=140.72ms
     http_req_receiving.........: avg=42.9µs   min=22µs     med=40.65µs  max=86.74µs  p(90)=57.7µs   p(95)=64.26µs 
     http_req_sending...........: avg=68.74µs  min=38.42µs  med=61.17µs  max=144.06µs p(90)=102.71µs p(95)=113.75µs
     http_req_tls_handshaking...: avg=3.8ms    min=0s       med=0s       max=35.93ms  p(90)=32.92ms  p(95)=34.2ms  
     http_req_waiting...........: avg=136.76ms min=131.07ms med=135.56ms max=215.35ms p(90)=139.09ms p(95)=140.63ms
     http_reqs..................: 90      8.650581/s
     iteration_duration.........: avg=1.14s    min=1.13s    med=1.13s    max=1.28s    p(90)=1.2s     p(95)=1.2s    
     iterations.................: 90      8.650581/s
     vus........................: 10      min=10     max=10
     vus_max....................: 10      min=10     max=10
```
Inspect examples folder for more details.

## More advanced use case

The above example shows how you can generate samples in JS and pass them into the extension, the extension will then submit them to the remote_write API endpoint.
When you want to produce samples at a very high rate the overhead which is involved when passing objects from the JS runtime to the extension can get expensive though,
to optimize this the extension also offers the option for the user to pass it a template based on which samples should be generated and then sent,
by generating the samples inside the extension the overhead of passing objects from the JS runtime to the extension can be avoided.

```javascript
const template = {
    __name__: 'k6_generated_metric_${series_id/4}',    // Name of the series.
    series_id: '${series_id}',                         // Each value of this label will match 1 series.
    cardinality_1e1: '${series_id/10}',                // Each value of this label will match 10 series.
    cardinality_2: '${series_id%2}',                   // Each value of this label will match 2 series.
};

write_client.storeFromTemplates(
    100,                // minimum random value
    200,                // maximum random value
    1643235433 * 1000,  // timestamp in ms
    42,                 // series id range start
    45,                 // series id range end
    template,
);
```

The above code could generate and send the following 3 samples, values are randomly chosen from the defined range:

```
Metric:    k6_generated_metric_10{cardinality_1e1="4", cardinality_2="0", series_id="42"},
Timestamp: 16432354331000
Value:     193
---
Metric:    k6_generated_metric_10{cardinality_1e1="4", cardinality_2="1", series_id="43"},
Timestamp: 16432354331000
Value:     121
---
Metric:    k6_generated_metric_11{cardinality_1e1="4", cardinality_2="0", series_id="44"},
Timestamp: 16432354331000
Value:     142
```
