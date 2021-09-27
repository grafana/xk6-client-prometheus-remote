# xk6-client-prometheus-remote

This is a [k6](https://go.k6.io/k6) extension developed using the [xk6](https://github.com/grafana/xk6) system.

## Build

To build a `k6` binary with this extension, first ensure you have the prerequisites:

- [Go toolchain](https://go101.org/article/go-toolchain.html)
- Git

Then:

1. Install `xk6`:
  ```shell
  $ go install go.k6.io/xk6/cmd/xk6@latest
  ```

2. Build the binary:
  ```shell
  $ xk6 build --with github.com/dgzlopes/xk6-remote-write@latest
  ```

## Example

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
        'is status 200': (r) => r.status_code === 200,
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

     checks......................: 100.00% ✓ 90       ✗ 0   
     data_received...............: 0 B     0 B/s
     data_sent...................: 6.2 kB  596 B/s
     iteration_duration..........: avg=1.14s    min=1.13s med=1.13s max=1.26s p(90)=1.23s p(95)=1.24s  
     iterations..................: 90      8.624555/s
     remote_write_num_series.....: 90      8.624555/s
     remote_write_req_duration...: avg=146.62ms min=129ms med=132ms max=260ms p(90)=231ms p(95)=242.1ms
     remote_write_reqs...........: 90      8.624555/s
     vus.........................: 10      min=10     max=10
     vus_max.....................: 10      min=10     max=10
```
