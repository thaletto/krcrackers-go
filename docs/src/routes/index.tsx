import { createFileRoute, Link } from '@tanstack/react-router';
import { HomeLayout } from 'fumadocs-ui/layouts/home';
import { baseOptions } from '@/lib/layout.shared';

export const Route = createFileRoute('/')({
  component: Home,
});

function Home() {
  return (
    <HomeLayout {...baseOptions()}>
      <div className="flex flex-col flex-1 justify-center px-4 py-8 text-center">
        <h1 className="font-medium text-2xl mb-2">KR Crackers</h1>
        <p className="text-fd-muted-foreground mb-6">
          Go HTTP API &middot; Cloudflare D1 &middot; Event-Driven Architecture
        </p>
        <Link
          to="/docs/$"
          params={{ _splat: '' }}
          className="px-4 py-2 rounded-lg bg-fd-primary text-fd-primary-foreground font-medium text-sm mx-auto"
        >
          Open Docs
        </Link>
      </div>
    </HomeLayout>
  );
}
