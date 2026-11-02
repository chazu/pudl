// Resource type registry: display metadata only (short code, label, visual
// class). Types are PUDL resource_type strings, matching the AWS and GCP
// content packs in examples/. Unknown types still render with a generic code.

export type ResourceClass =
  | 'admin' // org, folder, account, project
  | 'network' // VPC, subnet, gateway, route table, peering
  | 'compute'
  | 'data'
  | 'messaging'
  | 'edge' // load balancers, endpoints, serverless front doors
  | 'security' // SGs, firewalls, keys
  | 'identity' // users, roles, policies, service accounts
  | 'logical' // PUDL logical entities: applications, services
  | 'external';

export interface ResourceTypeInfo {
  type: string;
  /** 2–5 character code shown on the node. */
  code: string;
  label: string;
  cls: ResourceClass;
}

const R = (type: string, code: string, label: string, cls: ResourceClass): ResourceTypeInfo => ({
  type,
  code,
  label,
  cls,
});

const TYPES: ResourceTypeInfo[] = [
  R('aws.account', 'ACCT', 'AWS account', 'admin'),
  R('aws.ec2.vpc', 'VPC', 'VPC', 'network'),
  R('aws.ec2.subnet', 'SUBN', 'Subnet', 'network'),
  R('aws.ec2.internet_gateway', 'IGW', 'Internet gateway', 'network'),
  R('aws.ec2.nat_gateway', 'NAT', 'NAT gateway', 'network'),
  R('aws.ec2.route_table', 'RTB', 'Route table', 'network'),
  R('aws.ec2.vpc_peering', 'PCX', 'VPC peering', 'network'),
  R('aws.ec2.vpc_endpoint', 'VPCE', 'VPC endpoint', 'edge'),
  R('aws.ec2.network_interface', 'ENI', 'Network interface', 'network'),
  R('aws.ec2.instance', 'EC2', 'EC2 instance', 'compute'),
  R('aws.ec2.security_group', 'SG', 'Security group', 'security'),
  R('aws.elbv2.load_balancer', 'ALB', 'Load balancer', 'edge'),
  R('aws.rds.db_instance', 'RDS', 'RDS instance', 'data'),
  R('aws.s3.bucket', 'S3', 'S3 bucket', 'data'),
  R('aws.sqs.queue', 'SQS', 'SQS queue', 'messaging'),
  R('aws.kms.key', 'KMS', 'KMS key', 'security'),
  R('aws.iam.user', 'USER', 'IAM user', 'identity'),
  R('aws.iam.role', 'ROLE', 'IAM role', 'identity'),
  R('aws.iam.policy', 'POL', 'IAM policy', 'identity'),
  R('gcp.organization', 'ORG', 'Organization', 'admin'),
  R('gcp.folder', 'FLDR', 'Folder', 'admin'),
  R('gcp.project', 'PROJ', 'Project', 'admin'),
  R('gcp.compute.network', 'VPC', 'VPC network', 'network'),
  R('gcp.compute.subnetwork', 'SUBN', 'Subnetwork', 'network'),
  R('gcp.compute.firewall', 'FW', 'Firewall rule', 'security'),
  R('gcp.compute.router', 'RTR', 'Cloud Router', 'network'),
  R('gcp.compute.instance', 'GCE', 'Compute instance', 'compute'),
  R('gcp.sql.instance', 'SQL', 'Cloud SQL', 'data'),
  R('gcp.storage.bucket', 'GCS', 'Storage bucket', 'data'),
  R('gcp.pubsub.topic', 'TOPIC', 'Pub/Sub topic', 'messaging'),
  R('gcp.pubsub.subscription', 'SUB', 'Pub/Sub subscription', 'messaging'),
  R('gcp.run.service', 'RUN', 'Cloud Run service', 'edge'),
  R('gcp.iam.service_account', 'SA', 'Service account', 'identity'),
  R('external.cidr', 'CIDR', 'External address range', 'external'),
  R('external.principal', 'EXT', 'External principal', 'external'),
  R('logical.application', 'APP', 'Application', 'logical'),
  R('logical.service', 'SVC', 'Service', 'logical'),
];

const REGISTRY = new Map(TYPES.map((t) => [t.type, t]));

export function resourceType(type: string): ResourceTypeInfo {
  return (
    REGISTRY.get(type) ?? {
      type,
      code: type.split('.').pop()?.slice(0, 4).toUpperCase() ?? '?',
      label: type,
      cls: 'external',
    }
  );
}
