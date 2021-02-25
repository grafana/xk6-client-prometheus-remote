# xk6-remote-write

This is a [k6](https://github.com/loadimpact/k6) extension using the [xk6](https://github.com/k6io/xk6) system.

| :exclamation: This is a proof of concept, isn't supported by the k6 team, and may break in the future. USE AT YOUR OWN RISK! |
|------|

## Build

To build a `k6` binary with this extension, first ensure you have the prerequisites:

- [Go toolchain](https://go101.org/article/go-toolchain.html)
- Git

Then:

1. Clone `xk6`:
  ```shell
  git clone https://github.com/k6io/xk6.git
  cd xk6
  ```

2. Build the binary:
  ```shell
  CGO_ENABLED=1 go run ./cmd/xk6/main.go build master \
    --with github.com/dgzlopes/xk6-remote-write
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
    endpoint: "<your-remote-write-endpoint>"
});

export default function () {
    let res = client.storeNow({
        "__name__": `foo_bar${__VU}`,
        "foo": "bar"
    }, 12356)
    check(res, {
        'is status 200': (r) => r.status === 200,
    });
    sleep(1)
}
```

Result output:

```
$ ./k6 run example.js

          /\      |‾‾| /‾‾/   /‾‾/   
     /\  /  \     |  |/  /   /  /    
    /  \/    \    |     (   /   ‾‾\  
   /          \   |  |\  \ |  (‾)  | 
  / __________ \  |__| \__\ \_____/ .io

  execution: local
     script: ../example.js
     output: -

  scenarios: (100.00%) 1 scenario, 10 max VUs, 40s max duration (incl. graceful stop):
           * default: 10 looping VUs for 10s (gracefulStop: 30s)


running (10.4s), 00/10 VUs, 90 complete and 0 interrupted iterations
default ✓ [======================================] 10 VUs  10s

     ✓ is status 200

     checks......................: 100.00% ✓ 90   ✗ 0   
     data_received...............: 0 B     0 B/s
     data_sent...................: 18 EB   18 EB/s
     iteration_duration..........: avg=1.14s    min=1.12s med=1.13s max=1.25s p(90)=1.23s   p(95)=1.25s  
     iterations..................: 90      8.672493/s
     remote_write_req_duration...: avg=145.77ms min=129ms med=132ms max=253ms p(90)=236.1ms p(95)=251.1ms
     remote_write_req_total......: 90      8.672493/s
     vus.........................: 10      min=10 max=10
     vus_max.....................: 10      min=10 max=10
```
