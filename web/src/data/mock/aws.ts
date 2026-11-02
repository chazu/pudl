// AWS half of the demo dataset.
//
// Account 111111111111 (prod-app): us-east-1 production VPC with public, app
// and data tiers; us-west-2 DR VPC with a read replica.
// Account 222222222222 (shared-services): us-east-1 tooling VPC, peered with
// prod-core across accounts, running CI and monitoring.
//
// Two references deliberately point at resources that are not in the
// inventory (a security group in an uncollected account and an IAM role
// ARN), so the UI must show them as unresolved rather than invent them.

import { GraphBuilder, declared, inferred, observed } from './builder';

const EC2 = 'aws ec2 describe-instances';
const NET = 'aws ec2 describe-vpcs/subnets/route-tables';
const SG = 'aws ec2 describe-security-groups';
const IAM = 'aws iam get-account-authorization-details';
const ELB = 'aws elbv2 describe-target-health';
const FLOW = 'vpc flow logs (prod-core, 24h window)';
const MANIFEST = 'orders/deploy/services.yaml';

export function buildAws(b = new GraphBuilder('aws')): GraphBuilder {
  const A = '111111111111';
  const B = '222222222222';
  const E1 = 'us-east-1';
  const W2 = 'us-west-2';
  const ev = (s: string, d?: string) => [observed(s, d)];

  // --- accounts --------------------------------------------------------------
  const acctA = b.res({ id: `aws:account:${A}`, type: 'aws.account', name: 'prod-app', nativeId: A, account: A, evidence: ev('aws sts get-caller-identity') });
  const acctB = b.res({ id: `aws:account:${B}`, type: 'aws.account', name: 'shared-services', nativeId: B, account: B, evidence: ev('aws sts get-caller-identity') });

  const inAcct = (id: string, acct: string) => b.rel('in_account', id, acct, 'observed', { evidence: ev(NET) });

  // --- prod-core VPC (A, us-east-1) -----------------------------------------
  const vpc = b.res({ id: 'aws:vpc-0prodcore', type: 'aws.ec2.vpc', name: 'prod-core', nativeId: 'vpc-0prodcore', account: A, region: E1, properties: { CidrBlock: '10.10.0.0/16', IsDefault: false, State: 'available' }, tags: { env: 'prod' }, evidence: ev(NET) });
  inAcct(vpc, acctA);

  const subnet = (key: string, name: string, cidr: string, az: string, pub = false) => {
    const id = b.res({ id: `aws:subnet-0${key}`, type: 'aws.ec2.subnet', name, nativeId: `subnet-0${key}`, account: A, region: E1, zone: az, properties: { CidrBlock: cidr, AvailabilityZone: az, MapPublicIpOnLaunch: pub }, evidence: ev(NET) });
    b.rel('in_vpc', id, vpc, 'observed', { evidence: ev(NET) });
    return id;
  };
  const pubA = subnet('puba', 'public-a', '10.10.0.0/24', 'us-east-1a', true);
  const pubB = subnet('pubb', 'public-b', '10.10.1.0/24', 'us-east-1b', true);
  const appA = subnet('appa', 'app-a', '10.10.10.0/24', 'us-east-1a');
  const appB = subnet('appb', 'app-b', '10.10.11.0/24', 'us-east-1b');
  const dataA = subnet('dataa', 'data-a', '10.10.20.0/24', 'us-east-1a');
  // data-b holds the RDS standby, which the inventory does not model: an empty group.
  subnet('datab', 'data-b', '10.10.21.0/24', 'us-east-1b');

  const igw = b.res({ id: 'aws:igw-0prod', type: 'aws.ec2.internet_gateway', name: 'prod-igw', nativeId: 'igw-0prod', account: A, region: E1, evidence: ev(NET) });
  b.rel('attached_to', igw, vpc, 'observed', { evidence: ev(NET) });
  const nat = b.res({ id: 'aws:nat-0prod', type: 'aws.ec2.nat_gateway', name: 'prod-nat-a', nativeId: 'nat-0prod', account: A, region: E1, properties: { ConnectivityType: 'public', PublicIp: '54.0.0.10' }, evidence: ev(NET) });
  b.rel('in_subnet', nat, pubA, 'observed', { evidence: ev(NET) });

  const rtPub = b.res({ id: 'aws:rtb-0public', type: 'aws.ec2.route_table', name: 'rt-public', nativeId: 'rtb-0public', account: A, region: E1, evidence: ev(NET) });
  b.rel('in_vpc', rtPub, vpc, 'observed', { evidence: ev(NET) });
  b.rel('routes_via', rtPub, igw, 'observed', { properties: { destination: '0.0.0.0/0' }, evidence: ev(NET) });
  b.rel('associated_with', rtPub, pubA, 'observed', { evidence: ev(NET) });
  b.rel('associated_with', rtPub, pubB, 'observed', { evidence: ev(NET) });
  const rtApp = b.res({ id: 'aws:rtb-0private', type: 'aws.ec2.route_table', name: 'rt-private', nativeId: 'rtb-0private', account: A, region: E1, evidence: ev(NET) });
  b.rel('in_vpc', rtApp, vpc, 'observed', { evidence: ev(NET) });
  b.rel('routes_via', rtApp, nat, 'observed', { properties: { destination: '0.0.0.0/0' }, evidence: ev(NET) });
  b.rel('routes_via', rtApp, 'aws:pcx-0shared', 'observed', { properties: { destination: '10.50.0.0/16' }, evidence: ev(NET) });
  b.rel('associated_with', rtApp, appA, 'observed', { evidence: ev(NET) });
  b.rel('associated_with', rtApp, appB, 'observed', { evidence: ev(NET) });

  // Security groups
  const sg = (key: string, name: string, vpcId: string, acct: string, region: string) => {
    const id = b.res({ id: `aws:sg-0${key}`, type: 'aws.ec2.security_group', name, nativeId: `sg-0${key}`, account: acct, region, evidence: ev(SG) });
    b.rel('in_vpc', id, vpcId, 'observed', { evidence: ev(SG) });
    return id;
  };
  const sgAlb = sg('alb', 'orders-alb-sg', vpc, A, E1);
  const sgApi = sg('api', 'orders-api-sg', vpc, A, E1);
  const sgWorker = sg('worker', 'orders-worker-sg', vpc, A, E1);
  const sgDb = sg('db', 'orders-db-sg', vpc, A, E1);
  const sgBastion = sg('bastion', 'bastion-sg', vpc, A, E1);

  const internet = b.res({ id: 'ext:cidr:0.0.0.0/0', type: 'external.cidr', name: 'Internet (0.0.0.0/0)', provider: 'external', nativeId: '0.0.0.0/0', properties: { cidr: '0.0.0.0/0' } });
  const allow = (target: string, from: string, port: string) =>
    b.rel('allows_ingress_from', target, from, 'observed', { properties: { protocol: 'tcp', port }, evidence: ev(SG, `IpPermissions port ${port}`) });
  allow(sgAlb, internet, '443');
  allow(sgApi, sgAlb, '8080');
  allow(sgDb, sgApi, '5432');
  allow(sgDb, sgWorker, '5432');
  allow(sgBastion, internet, '22');
  allow(sgApi, sgBastion, '22');
  // Rule references a group in an account PUDL has not collected.
  allow(sgApi, 'aws:sg-0deadbeef', '9100');

  // Instances, each with a primary ENI (rendered as a port in some views).
  const instance = (key: string, name: string, sub: string, sgs: string[], props: Record<string, unknown> = {}, acct = A, region = E1) => {
    const id = b.res({ id: `aws:i-0${key}`, type: 'aws.ec2.instance', name, nativeId: `i-0${key}`, account: acct, region, properties: { InstanceType: 't3.large', State: 'running', ...props }, tags: { Name: name }, evidence: ev(EC2) });
    b.rel('in_subnet', id, sub, 'observed', { evidence: ev(EC2, 'SubnetId') });
    const eni = b.res({ id: `aws:eni-0${key}`, type: 'aws.ec2.network_interface', name: `${name} eth0`, nativeId: `eni-0${key}`, account: acct, region, properties: { DeviceIndex: 0 }, evidence: ev(EC2, 'NetworkInterfaces[0]') });
    b.rel('attached_to', eni, id, 'observed', { properties: { deviceIndex: 0 }, evidence: ev(EC2) });
    b.rel('in_subnet', eni, sub, 'observed', { evidence: ev(EC2) });
    for (const g of sgs) b.rel('protected_by', eni, g, 'observed', { evidence: ev(EC2, 'NetworkInterfaces[0].Groups') });
    return { id, eni };
  };
  const api1 = instance('api1', 'orders-api-1', appA, [sgApi], { PrivateIpAddress: '10.10.10.11' });
  const api2 = instance('api2', 'orders-api-2', appB, [sgApi], { PrivateIpAddress: '10.10.11.12' });
  const worker1 = instance('wrk1', 'orders-worker-1', appA, [sgWorker], { InstanceType: 'c6i.xlarge' });
  const bastion = instance('bast', 'bastion', pubA, [sgBastion], { InstanceType: 't3.micro', PublicIpAddress: '54.0.0.22' });

  // api-1 has a second interface in the data tier: two semantic ports, two subnets.
  const api1Eth1 = b.res({ id: 'aws:eni-0api1b', type: 'aws.ec2.network_interface', name: 'orders-api-1 eth1', nativeId: 'eni-0api1b', account: A, region: E1, properties: { DeviceIndex: 1 }, evidence: ev(EC2, 'NetworkInterfaces[1]') });
  b.rel('attached_to', api1Eth1, api1.id, 'observed', { properties: { deviceIndex: 1 }, evidence: ev(EC2) });
  b.rel('in_subnet', api1Eth1, dataA, 'observed', { evidence: ev(EC2) });
  b.rel('protected_by', api1Eth1, sgApi, 'observed', { evidence: ev(EC2) });

  const alb = b.res({ id: 'aws:alb-orders', type: 'aws.elbv2.load_balancer', name: 'orders-alb', nativeId: `arn:aws:elasticloadbalancing:${E1}:${A}:loadbalancer/app/orders-alb/50dc6c495c0c9188`, account: A, region: E1, properties: { Scheme: 'internet-facing', Type: 'application', DNSName: 'orders-alb-123.us-east-1.elb.amazonaws.com' }, evidence: ev('aws elbv2 describe-load-balancers') });
  b.rel('in_subnet', alb, pubA, 'observed', { evidence: ev('aws elbv2 describe-load-balancers', 'AvailabilityZones') });
  b.rel('in_subnet', alb, pubB, 'observed', { evidence: ev('aws elbv2 describe-load-balancers', 'AvailabilityZones') });
  b.rel('protected_by', alb, sgAlb, 'observed', { evidence: ev('aws elbv2 describe-load-balancers') });
  b.rel('exposed_to', alb, internet, 'observed', { properties: { listener: 'HTTPS:443' }, evidence: ev('aws elbv2 describe-listeners') });
  // Same pair, several relationships: target registration and health checks.
  for (const t of [api1.id, api2.id]) {
    b.rel('routes_to', alb, t, 'observed', { properties: { targetGroup: 'orders-api-tg', port: 8080 }, evidence: ev(ELB) });
    b.rel('health_checks', alb, t, 'observed', { properties: { path: '/healthz', state: 'healthy' }, evidence: ev(ELB) });
  }

  const db = b.res({ id: 'aws:rds-orders', type: 'aws.rds.db_instance', name: 'orders-db', nativeId: `arn:aws:rds:${E1}:${A}:db:orders-db`, account: A, region: E1, zone: 'us-east-1a', properties: { Engine: 'postgres', EngineVersion: '16.4', MultiAZ: true, DBInstanceClass: 'db.r6g.large' }, evidence: ev('aws rds describe-db-instances') });
  b.rel('in_subnet', db, dataA, 'observed', { evidence: ev('aws rds describe-db-instances', 'primary AZ us-east-1a; standby in data-b not modeled') });
  b.rel('protected_by', db, sgDb, 'observed', { evidence: ev('aws rds describe-db-instances', 'VpcSecurityGroups') });

  const queue = b.res({ id: 'aws:sqs-orders', type: 'aws.sqs.queue', name: 'orders-queue', nativeId: `arn:aws:sqs:${E1}:${A}:orders-queue`, account: A, region: E1, properties: { VisibilityTimeout: 60, FifoQueue: false }, evidence: ev('aws sqs list-queues') });
  inAcct(queue, acctA);
  const bucket = b.res({ id: 'aws:s3-orders-artifacts', type: 'aws.s3.bucket', name: 'orders-artifacts', nativeId: 'arn:aws:s3:::orders-artifacts', account: A, region: E1, properties: { Versioning: 'Enabled', PublicAccessBlock: true }, evidence: ev('aws s3api list-buckets') });
  inAcct(bucket, acctA);
  const kms = b.res({ id: 'aws:kms-orders', type: 'aws.kms.key', name: 'orders-data-key', nativeId: `arn:aws:kms:${E1}:${A}:key/1234abcd`, account: A, region: E1, evidence: ev('aws kms describe-key') });
  inAcct(kms, acctA);
  b.rel('encrypted_by', db, kms, 'observed', { evidence: ev('aws rds describe-db-instances', 'KmsKeyId') });
  b.rel('encrypted_by', bucket, kms, 'observed', { evidence: ev('aws s3api get-bucket-encryption') });

  const vpce = b.res({ id: 'aws:vpce-0sqs', type: 'aws.ec2.vpc_endpoint', name: 'sqs-endpoint', nativeId: 'vpce-0sqs', account: A, region: E1, properties: { VpcEndpointType: 'Interface', ServiceName: 'com.amazonaws.us-east-1.sqs' }, evidence: ev('aws ec2 describe-vpc-endpoints') });
  b.rel('in_subnet', vpce, appA, 'observed', { evidence: ev('aws ec2 describe-vpc-endpoints') });
  b.rel('in_subnet', vpce, appB, 'observed', { evidence: ev('aws ec2 describe-vpc-endpoints') });
  b.rel('endpoint_for', vpce, queue, 'observed', { evidence: ev('aws ec2 describe-vpc-endpoints', 'ServiceName') });

  // Application dependencies (declared by the deploy manifest).
  for (const api of [api1.id, api2.id]) {
    b.rel('reads_from', api, db, 'declared', { properties: { via: 'DATABASE_URL' }, evidence: [declared(MANIFEST, 'orders-api env')] });
    b.rel('writes_to', api, db, 'declared', { evidence: [declared(MANIFEST, 'orders-api env')] });
    b.rel('publishes_to', api, queue, 'declared', { evidence: [declared(MANIFEST, 'ORDERS_QUEUE_URL')] });
  }
  b.rel('consumes_from', worker1.id, queue, 'declared', { evidence: [declared(MANIFEST, 'orders-worker')] });
  b.rel('writes_to', worker1.id, db, 'declared', { evidence: [declared(MANIFEST, 'orders-worker')] });
  b.rel('writes_to', worker1.id, bucket, 'declared', { evidence: [declared(MANIFEST, 'ARTIFACT_BUCKET')] });
  // Observed communication, distinct from the declared dependency above.
  b.rel('observed_flow', api1.id, db, 'observed', { properties: { port: 5432, packets: 18234 }, evidence: [observed(FLOW, '10.10.10.11 -> 10.10.20.5:5432')] });

  // --- IAM (A) ----------------------------------------------------------------
  const role = (name: string, acct: string, props: Record<string, unknown> = {}) => {
    const id = b.res({ id: `aws:role:${acct}:${name}`, type: 'aws.iam.role', name, nativeId: `arn:aws:iam::${acct}:role/${name}`, account: acct, properties: props, evidence: ev(IAM) });
    b.rel('in_account', id, `aws:account:${acct}`, 'observed', { evidence: ev(IAM) });
    return id;
  };
  const policy = (name: string, acct: string) => {
    const id = b.res({ id: `aws:policy:${acct}:${name}`, type: 'aws.iam.policy', name, nativeId: `arn:aws:iam::${acct}:policy/${name}`, account: acct, evidence: ev(IAM) });
    b.rel('in_account', id, `aws:account:${acct}`, 'observed', { evidence: ev(IAM) });
    return id;
  };
  const apiRole = role('orders-api-role', A);
  const workerRole = role('orders-worker-role', A);
  const deployRole = role('orders-deploy', A, { MaxSessionDuration: 3600 });
  const apiPolicy = policy('orders-api-access', A);
  const workerPolicy = policy('orders-worker-access', A);
  const deployPolicy = policy('orders-deploy-access', A);
  for (const api of [api1.id, api2.id]) b.rel('has_instance_profile', api, apiRole, 'observed', { evidence: ev(EC2, 'IamInstanceProfile') });
  b.rel('has_instance_profile', worker1.id, workerRole, 'observed', { evidence: ev(EC2, 'IamInstanceProfile') });
  b.rel('has_policy', apiRole, apiPolicy, 'observed', { evidence: ev(IAM, 'AttachedManagedPolicies') });
  b.rel('has_policy', workerRole, workerPolicy, 'observed', { evidence: ev(IAM, 'AttachedManagedPolicies') });
  b.rel('has_policy', deployRole, deployPolicy, 'observed', { evidence: ev(IAM, 'AttachedManagedPolicies') });
  b.rel('grants_on', apiPolicy, queue, 'observed', { properties: { actions: ['sqs:SendMessage'] }, evidence: ev(IAM, 'Statement[0]') });
  b.rel('grants_on', workerPolicy, queue, 'observed', { properties: { actions: ['sqs:ReceiveMessage', 'sqs:DeleteMessage'] }, evidence: ev(IAM, 'Statement[0]') });
  b.rel('grants_on', workerPolicy, bucket, 'observed', { properties: { actions: ['s3:PutObject'] }, evidence: ev(IAM, 'Statement[1]') });
  b.rel('grants_on', workerPolicy, kms, 'observed', { properties: { actions: ['kms:GenerateDataKey'] }, evidence: ev(IAM, 'Statement[2]') });
  b.rel('grants_on', deployPolicy, bucket, 'observed', { properties: { actions: ['s3:*'] }, evidence: ev(IAM, 'Statement[0]') });
  const breakGlass = b.res({ id: `aws:user:${A}:break-glass`, type: 'aws.iam.user', name: 'break-glass', nativeId: `arn:aws:iam::${A}:user/break-glass`, account: A, evidence: ev(IAM) });
  b.rel('in_account', breakGlass, acctA, 'observed', { evidence: ev(IAM) });
  b.rel('trust_allows_assume', breakGlass, deployRole, 'observed', { evidence: ev(IAM, 'AssumeRolePolicyDocument') });
  // Instance profile ARN that PUDL never collected.
  b.rel('has_instance_profile', bastion.id, `aws:role:${A}:bastion-legacy`, 'observed', { evidence: ev(EC2, 'IamInstanceProfile (role not in inventory)') });

  // --- prod-dr VPC (A, us-west-2) --------------------------------------------
  const vpcDr = b.res({ id: 'aws:vpc-0proddr', type: 'aws.ec2.vpc', name: 'prod-dr', nativeId: 'vpc-0proddr', account: A, region: W2, properties: { CidrBlock: '10.20.0.0/16' }, evidence: ev(NET) });
  inAcct(vpcDr, acctA);
  const drApp = b.res({ id: 'aws:subnet-0drapp', type: 'aws.ec2.subnet', name: 'dr-app-a', nativeId: 'subnet-0drapp', account: A, region: W2, zone: 'us-west-2a', properties: { CidrBlock: '10.20.10.0/24' }, evidence: ev(NET) });
  b.rel('in_vpc', drApp, vpcDr, 'observed', { evidence: ev(NET) });
  const drData = b.res({ id: 'aws:subnet-0drdata', type: 'aws.ec2.subnet', name: 'dr-data-a', nativeId: 'subnet-0drdata', account: A, region: W2, zone: 'us-west-2a', properties: { CidrBlock: '10.20.20.0/24' }, evidence: ev(NET) });
  b.rel('in_vpc', drData, vpcDr, 'observed', { evidence: ev(NET) });
  const sgDr = sg('dr', 'orders-dr-sg', vpcDr, A, W2);
  const drApi = instance('drapi1', 'orders-api-dr-1', drApp, [sgDr], { State: 'stopped' }, A, W2);
  const replica = b.res({ id: 'aws:rds-orders-replica', type: 'aws.rds.db_instance', name: 'orders-db-replica', nativeId: `arn:aws:rds:${W2}:${A}:db:orders-db-replica`, account: A, region: W2, properties: { Engine: 'postgres', ReadReplicaSource: 'orders-db' }, evidence: ev('aws rds describe-db-instances') });
  b.rel('in_subnet', replica, drData, 'observed', { evidence: ev('aws rds describe-db-instances') });
  b.rel('replica_of', replica, db, 'observed', { properties: { lagSeconds: 4 }, evidence: ev('aws rds describe-db-instances', 'ReadReplicaSourceDBInstanceIdentifier') });
  b.rel('reads_from', drApi.id, replica, 'declared', { evidence: [declared(MANIFEST, 'orders-api-dr env')] });

  // --- shared-services VPC (B, us-east-1) ------------------------------------
  const vpcShared = b.res({ id: 'aws:vpc-0shared', type: 'aws.ec2.vpc', name: 'shared-svcs', nativeId: 'vpc-0shared', account: B, region: E1, properties: { CidrBlock: '10.50.0.0/16' }, evidence: ev(NET) });
  b.rel('in_account', vpcShared, acctB, 'observed', { evidence: ev(NET) });
  const toolsA = b.res({ id: 'aws:subnet-0toolsa', type: 'aws.ec2.subnet', name: 'tools-a', nativeId: 'subnet-0toolsa', account: B, region: E1, zone: 'us-east-1a', properties: { CidrBlock: '10.50.1.0/24' }, evidence: ev(NET) });
  b.rel('in_vpc', toolsA, vpcShared, 'observed', { evidence: ev(NET) });
  const sgTools = sg('tools', 'tools-sg', vpcShared, B, E1);
  const ci = instance('ci1', 'ci-runner-1', toolsA, [sgTools], { InstanceType: 'c6i.2xlarge' }, B, E1);
  const mon = instance('mon1', 'prometheus-1', toolsA, [sgTools], { InstanceType: 'r6i.large' }, B, E1);
  // Cross-account SG reference across the peering.
  allow(sgApi, sgTools, '9100');

  const pcx = b.res({ id: 'aws:pcx-0shared', type: 'aws.ec2.vpc_peering', name: 'prod-core ⇄ shared-svcs', nativeId: 'pcx-0shared', account: A, region: E1, properties: { Status: 'active', RequesterOwner: A, AccepterOwner: B }, evidence: ev('aws ec2 describe-vpc-peering-connections') });
  b.rel('peers_with', pcx, vpc, 'observed', { properties: { side: 'requester' }, evidence: ev('aws ec2 describe-vpc-peering-connections') });
  b.rel('peers_with', pcx, vpcShared, 'observed', { properties: { side: 'accepter' }, evidence: ev('aws ec2 describe-vpc-peering-connections') });

  for (const api of [api1.id, api2.id]) {
    b.rel('scrapes', mon.id, api, 'declared', { properties: { port: 9100, job: 'node' }, evidence: [declared('prometheus/prometheus.yml', 'scrape_configs[node]')] });
  }

  const ciRole = role('ci-runner', B);
  b.rel('has_instance_profile', ci.id, ciRole, 'observed', { evidence: ev(EC2, 'IamInstanceProfile') });
  // Cross-account trust: prod-app's deploy role trusts shared-services' CI role.
  b.rel('trust_allows_assume', ciRole, deployRole, 'observed', { properties: { condition: 'sts:ExternalId present' }, evidence: ev(IAM, `orders-deploy AssumeRolePolicyDocument names arn:aws:iam::${B}:role/ci-runner`) });

  // Inferred, weakest evidence: bastion grouped with orders by tag only.
  b.rel('member_of', bastion.id, 'logical:svc:orders-api', 'inferred', { evidence: [inferred('tag rule app=orders', 'Tag app=orders on bastion')] });

  return b;
}
