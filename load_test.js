// load_test.js
import http from 'k6/http';
import { check, sleep } from 'k6';

export const options = {
  stages: [
    { duration: '30s', target: 500 },
    { duration: '1m', target: 500 },
    { duration: '30s', target: 0 },
  ],
  thresholds: {
    'http_req_duration': ['p(95)<50'],
    'http_req_failed': ['rate<0.01'],
  },
};

export default function () {
  const res = http.get('http://app:8080/stats/assignments');
  
  check(res, { 'status was 200': (r) => r.status === 200 });
  sleep(0.1);
}
