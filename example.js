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
        "foo": "bar",
    }, Math.random() * 100)
    check(res, {
        'is status 200': (r) => r.status === 200,
    });
    sleep(1)
}