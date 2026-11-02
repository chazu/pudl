// GCP half of the demo dataset.
//
// Organization example.com → folder production → projects acme-net-host
// (Shared VPC host) and acme-analytics (service project). The ETL instances
// belong to acme-analytics but sit in the host project's subnets, so the
// network view places them under acme-net-host / shared-vpc / region / subnet
// (six levels deep) while the inventory view places them under their own
// project. VPC networks are global and subnets regional, unlike AWS.

import { GraphBuilder, declared, observed } from './builder';

const ASSET = 'gcloud asset search-all-resources';
const COMPUTE = 'gcloud compute instances list';
const FW = 'gcloud compute firewall-rules list';
const IAMP = 'gcloud projects get-iam-policy';
const PIPE = 'analytics/pipeline.cue';

export function buildGcp(b = new GraphBuilder('gcp')): GraphBuilder {
  const ev = (s: string, d?: string) => [observed(s, d)];
  const HOST = 'acme-net-host';
  const SVC = 'acme-analytics';

  const org = b.res({ id: 'gcp:org:example.com', type: 'gcp.organization', name: 'example.com', nativeId: 'organizations/482910374651', evidence: ev(ASSET) });
  const folder = b.res({ id: 'gcp:folder:production', type: 'gcp.folder', name: 'production', nativeId: 'folders/90817263', evidence: ev(ASSET) });
  b.rel('in_organization', folder, org, 'observed', { evidence: ev(ASSET) });
  const project = (id: string, num: string) => {
    const pid = b.res({ id: `gcp:project:${id}`, type: 'gcp.project', name: id, nativeId: id, account: id, properties: { projectNumber: num }, evidence: ev('gcloud projects list') });
    b.rel('in_folder', pid, folder, 'observed', { evidence: ev(ASSET) });
    return pid;
  };
  const host = project(HOST, '100200300400');
  const svc = project(SVC, '500600700800');

  const net = b.res({ id: `gcp:network:${HOST}/shared-vpc`, type: 'gcp.compute.network', name: 'shared-vpc', nativeId: `projects/${HOST}/global/networks/shared-vpc`, account: HOST, properties: { routingMode: 'GLOBAL', autoCreateSubnetworks: false, sharedVpcHost: true }, evidence: ev('gcloud compute networks list') });
  b.rel('in_project', net, host, 'observed', { evidence: ev(ASSET) });

  const subnet = (name: string, region: string, cidr: string) => {
    const id = b.res({ id: `gcp:subnet:${HOST}/${region}/${name}`, type: 'gcp.compute.subnetwork', name, nativeId: `projects/${HOST}/regions/${region}/subnetworks/${name}`, account: HOST, region, properties: { ipCidrRange: cidr, privateIpGoogleAccess: true }, evidence: ev('gcloud compute networks subnets list') });
    b.rel('in_network', id, net, 'observed', { evidence: ev('gcloud compute networks subnets list') });
    return id;
  };
  const usc1 = subnet('analytics-usc1', 'us-central1', '10.100.0.0/20');
  const euw1 = subnet('analytics-euw1', 'europe-west1', '10.110.0.0/20');

  const router = b.res({ id: `gcp:router:${HOST}/us-central1/nat-router`, type: 'gcp.compute.router', name: 'nat-router-usc1', nativeId: `projects/${HOST}/regions/us-central1/routers/nat-router-usc1`, account: HOST, region: 'us-central1', properties: { nat: 'AUTO_ONLY' }, evidence: ev('gcloud compute routers list') });
  b.rel('in_network', router, net, 'observed', { evidence: ev('gcloud compute routers list') });

  // Service accounts
  const sa = (name: string, proj: string, projId: string) => {
    const id = b.res({ id: `gcp:sa:${name}@${proj}`, type: 'gcp.iam.service_account', name: `${name}@${proj}`, nativeId: `${name}@${proj}.iam.gserviceaccount.com`, account: proj, evidence: ev('gcloud iam service-accounts list') });
    b.rel('in_project', id, projId, 'observed', { evidence: ev(ASSET) });
    return id;
  };
  const etlSa = sa('etl', SVC, svc);
  const ingestSa = sa('ingest', SVC, svc);

  // ETL instances: owned by the service project, addressed in host subnets.
  const vm = (name: string, sub: string, region: string, zone: string) => {
    const id = b.res({ id: `gcp:instance:${SVC}/${zone}/${name}`, type: 'gcp.compute.instance', name, nativeId: `projects/${SVC}/zones/${zone}/instances/${name}`, account: SVC, region, zone, properties: { machineType: 'n2-standard-4', status: 'RUNNING', networkTags: ['etl'] }, evidence: ev(COMPUTE) });
    b.rel('in_project', id, svc, 'observed', { evidence: ev(COMPUTE) });
    b.rel('in_subnet', id, sub, 'observed', { evidence: ev(COMPUTE, 'networkInterfaces[0].subnetwork (Shared VPC)') });
    b.rel('runs_as', id, etlSa, 'observed', { evidence: ev(COMPUTE, 'serviceAccounts[0]') });
    return id;
  };
  const etl1 = vm('etl-runner-1', usc1, 'us-central1', 'us-central1-a');
  const etl2 = vm('etl-runner-2', euw1, 'europe-west1', 'europe-west1-b');

  const iapRange = b.res({ id: 'ext:cidr:35.235.240.0/20', type: 'external.cidr', name: 'IAP TCP forwarding (35.235.240.0/20)', provider: 'external', nativeId: '35.235.240.0/20', properties: { cidr: '35.235.240.0/20' } });
  const fw = (name: string, props: Record<string, unknown>) => {
    const id = b.res({ id: `gcp:firewall:${HOST}/${name}`, type: 'gcp.compute.firewall', name, nativeId: `projects/${HOST}/global/firewalls/${name}`, account: HOST, properties: props, evidence: ev(FW) });
    b.rel('in_network', id, net, 'observed', { evidence: ev(FW) });
    return id;
  };
  const fwSsh = fw('allow-iap-ssh', { direction: 'INGRESS', allowed: 'tcp:22', targetTags: ['etl'] });
  const fwDeny = fw('deny-all-ingress', { direction: 'INGRESS', denied: 'all', priority: 65534 });
  b.rel('allows_ingress_from', fwSsh, iapRange, 'observed', { properties: { protocol: 'tcp', port: '22' }, evidence: ev(FW, 'sourceRanges') });
  for (const v of [etl1, etl2]) {
    b.rel('applies_to', fwSsh, v, 'observed', { properties: { matchedBy: 'targetTags: etl' }, evidence: ev(FW, 'targetTags matched instance tags') });
    b.rel('applies_to', fwDeny, v, 'observed', { properties: { matchedBy: 'all instances' }, evidence: ev(FW) });
  }

  // Managed services in the service project.
  const sql = b.res({ id: `gcp:sql:${SVC}/analytics-pg`, type: 'gcp.sql.instance', name: 'analytics-pg', nativeId: `projects/${SVC}/instances/analytics-pg`, account: SVC, region: 'us-central1', properties: { databaseVersion: 'POSTGRES_16', tier: 'db-custom-4-16384', ipv4Enabled: false }, evidence: ev('gcloud sql instances list') });
  b.rel('in_project', sql, svc, 'observed', { evidence: ev(ASSET) });
  b.rel('private_ip_in', sql, net, 'observed', { properties: { allocatedRange: 'google-managed-services-shared-vpc' }, evidence: ev('gcloud sql instances describe', 'ipConfiguration.privateNetwork') });

  const raw = b.res({ id: `gcp:bucket:${SVC}-raw`, type: 'gcp.storage.bucket', name: `${SVC}-raw`, nativeId: `gs://${SVC}-raw`, account: SVC, region: 'us-central1', properties: { location: 'US-CENTRAL1', uniformBucketLevelAccess: true }, evidence: ev('gcloud storage buckets list') });
  b.rel('in_project', raw, svc, 'observed', { evidence: ev(ASSET) });
  const topic = b.res({ id: `gcp:topic:${SVC}/events`, type: 'gcp.pubsub.topic', name: 'events', nativeId: `projects/${SVC}/topics/events`, account: SVC, evidence: ev('gcloud pubsub topics list') });
  b.rel('in_project', topic, svc, 'observed', { evidence: ev(ASSET) });
  const sub = b.res({ id: `gcp:subscription:${SVC}/events-etl`, type: 'gcp.pubsub.subscription', name: 'events-etl', nativeId: `projects/${SVC}/subscriptions/events-etl`, account: SVC, properties: { ackDeadlineSeconds: 60 }, evidence: ev('gcloud pubsub subscriptions list') });
  b.rel('in_project', sub, svc, 'observed', { evidence: ev(ASSET) });
  b.rel('subscribed_to', sub, topic, 'observed', { evidence: ev('gcloud pubsub subscriptions describe', 'topic') });
  const run = b.res({ id: `gcp:run:${SVC}/us-central1/ingest-api`, type: 'gcp.run.service', name: 'ingest-api', nativeId: `projects/${SVC}/locations/us-central1/services/ingest-api`, account: SVC, region: 'us-central1', properties: { ingress: 'all', minInstances: 1 }, evidence: ev('gcloud run services list') });
  b.rel('in_project', run, svc, 'observed', { evidence: ev(ASSET) });
  b.rel('runs_as', run, ingestSa, 'observed', { evidence: ev('gcloud run services describe', 'serviceAccountName') });
  const anyone = b.res({ id: 'ext:principal:allUsers', type: 'external.principal', name: 'allUsers', provider: 'external', nativeId: 'allUsers', properties: { note: 'Any internet caller' } });
  b.rel('iam_binding', anyone, run, 'observed', { properties: { role: 'roles/run.invoker' }, evidence: ev('gcloud run services get-iam-policy') });

  // IAM bindings (resource-level).
  b.rel('iam_binding', etlSa, sub, 'observed', { properties: { role: 'roles/pubsub.subscriber' }, evidence: ev('gcloud pubsub subscriptions get-iam-policy') });
  b.rel('iam_binding', etlSa, raw, 'observed', { properties: { role: 'roles/storage.objectViewer' }, evidence: ev('gcloud storage buckets get-iam-policy') });
  b.rel('iam_binding', ingestSa, topic, 'observed', { properties: { role: 'roles/pubsub.publisher' }, evidence: ev('gcloud pubsub topics get-iam-policy') });
  b.rel('iam_binding', etlSa, svc, 'observed', { properties: { role: 'roles/cloudsql.client' }, evidence: ev(IAMP, 'project-level binding') });
  // Shared VPC: the service project's SA can use host subnets.
  b.rel('iam_binding', etlSa, usc1, 'observed', { properties: { role: 'roles/compute.networkUser' }, evidence: ev('gcloud compute networks subnets get-iam-policy') });

  // Pipeline dependencies (declared).
  b.rel('publishes_to', run, topic, 'declared', { evidence: [declared(PIPE, 'ingest.output')] });
  for (const v of [etl1, etl2]) {
    b.rel('consumes_from', v, sub, 'declared', { evidence: [declared(PIPE, 'etl.input')] });
    b.rel('reads_from', v, raw, 'declared', { evidence: [declared(PIPE, 'etl.lookup')] });
    b.rel('writes_to', v, sql, 'declared', { evidence: [declared(PIPE, 'etl.sink')] });
  }
  return b;
}
