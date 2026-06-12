import { openapi } from '@/lib/openapi';
import { createFileRoute } from '@tanstack/react-router';

const proxy = openapi.createProxy({
  allowedOrigins: ['http://localhost:3000', 'http://localhost:8080'],
});

export const Route = createFileRoute('/api/proxy')({
  server: {
    handlers: {
      GET: async ({ request }: { request: Request }) => proxy.handle(request),
      POST: async ({ request }: { request: Request }) => proxy.handle(request),
      PUT: async ({ request }: { request: Request }) => proxy.handle(request),
      PATCH: async ({ request }: { request: Request }) => proxy.handle(request),
      DELETE: async ({ request }: { request: Request }) => proxy.handle(request),
    },
  },
});
