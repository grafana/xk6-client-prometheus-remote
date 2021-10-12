import { check, sleep } from 'k6';
import remote from 'k6/x/remotewrite';

export let options = {
    vus: 10,
    duration: '5m',
    ext: {
        loadimpact: {
          projectID: 3183896,
          // Test runs with the same name groups test runs together
          name: "Prometheus Remote Write Client"
        }
      }
};

const client = new remote.Client({
    url: "<remote-write-URL>"
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