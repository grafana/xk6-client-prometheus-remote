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
    url: "https://12340:eyJrIjoiZGM4YTI5MjQ5NmY1MDM4OGNmZTViNjk2ZWNjYjgzZDM1YTY2MDEyMCIsIm4iOiJHcmFmYW5hIENsb3VkIE1ldHJpY3MgUHVibGlzaCBTdGFnaW5nIiwiaWQiOjM5NzU0NX0=@prometheus-us-central1.grafana.net/api/prom/push"
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