// Logical entities: applications and services. These are not provider
// resources; they come from declared manifests (here, the demo's own), and
// their membership relationships are declared or inferred, never observed.

import { GraphBuilder, declared } from './builder';

export function buildApplications(b = new GraphBuilder('logical')): GraphBuilder {
  const app = (key: string, name: string, owner: string) =>
    b.res({ id: `logical:app:${key}`, type: 'logical.application', name, properties: { owner }, evidence: [declared('pudl/apps.cue')] });
  const svc = (key: string, name: string, appId: string) => {
    const id = b.res({ id: `logical:svc:${key}`, type: 'logical.service', name, evidence: [declared('pudl/apps.cue')] });
    b.rel('part_of', id, appId, 'declared', { evidence: [declared('pudl/apps.cue')] });
    return id;
  };
  const member = (resource: string, service: string) =>
    b.rel('member_of', resource, service, 'declared', { evidence: [declared('pudl/apps.cue', 'services.members')] });

  const orders = app('orders', 'Orders', 'team-commerce');
  const frontend = svc('orders-frontend', 'orders-frontend', orders);
  const api = svc('orders-api', 'orders-api', orders);
  const worker = svc('orders-worker', 'orders-worker', orders);
  const data = svc('orders-data', 'orders-data', orders);
  member('aws:alb-orders', frontend);
  member('aws:i-0api1', api);
  member('aws:i-0api2', api);
  member('aws:i-0drapi1', api);
  member('aws:i-0wrk1', worker);
  member('aws:rds-orders', data);
  member('aws:rds-orders-replica', data);
  member('aws:sqs-orders', data);
  member('aws:s3-orders-artifacts', data);

  const analytics = app('analytics', 'Analytics', 'team-data');
  const ingest = svc('ingest', 'ingest', analytics);
  const etl = svc('etl', 'etl', analytics);
  const warehouse = svc('warehouse', 'warehouse', analytics);
  const SVC = 'acme-analytics';
  member(`gcp:run:${SVC}/us-central1/ingest-api`, ingest);
  member(`gcp:topic:${SVC}/events`, ingest);
  member(`gcp:subscription:${SVC}/events-etl`, etl);
  member(`gcp:instance:${SVC}/us-central1-a/etl-runner-1`, etl);
  member(`gcp:instance:${SVC}/europe-west1-b/etl-runner-2`, etl);
  member(`gcp:sql:${SVC}/analytics-pg`, warehouse);
  member(`gcp:bucket:${SVC}-raw`, warehouse);

  const platform = app('platform', 'Platform tooling', 'team-platform');
  const ci = svc('ci', 'ci', platform);
  const observability = svc('observability', 'observability', platform);
  member('aws:i-0ci1', ci);
  member('aws:i-0mon1', observability);
  return b;
}
