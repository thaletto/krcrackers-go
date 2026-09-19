import http from 'k6/http';
import { check } from 'k6';

// Load test for KR Crackers read paths (no writes — safe against dev data).
// Usage: BASE_URL=http://localhost:8080 k6 run scripts/load.js
// Or:    make load [BASE_URL=http://localhost:8080]

export const options = {
  stages: [
    { duration: '30s', target: 20 },
    { duration: '1m', target: 50 },
    { duration: '30s', target: 0 },
  ],
  thresholds: {
    http_req_failed: ['rate<0.01'],
    http_req_duration: ['p(95)<500'],
  },
};

const BASE = __ENV.BASE_URL || 'http://localhost:8080';

const ENDPOINTS = [
  '/health',
  '/products?limit=20',
  '/products?q=cracker&limit=20',
  '/products?category=Gift%20Boxes&limit=20',
  '/orders?limit=20',
];

export default function () {
  for (const path of ENDPOINTS) {
    const res = http.get(`${BASE}${path}`);
    check(res, {
      [`GET ${path} no 5xx`]: (r) => r.status < 500,
    });
  }
}
