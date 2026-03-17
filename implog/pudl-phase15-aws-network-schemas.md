# pudl-phase15: AWS Network Schemas

## Summary

Added `pudl/aws` bootstrap schema package with 11 resource schemas modeling AWS networking primitives. Fields use PascalCase matching AWS CLI/SDK output so that `pudl import` on raw `aws ec2 describe-*` JSON auto-infers the correct schema.

## Schemas Added

| Schema | Resource Type | Identity | Tracked Fields |
|--------|--------------|----------|----------------|
| `#Account` | `aws.account` | `Id` | `Arn`, `Name`, `Status`, `Email` |
| `#VPC` | `aws.ec2.vpc` | `VpcId` | `CidrBlock`, `State`, `IsDefault`, `EnableDnsSupport`, `EnableDnsHostnames` |
| `#Subnet` | `aws.ec2.subnet` | `SubnetId` | `VpcId`, `CidrBlock`, `AvailabilityZone`, `MapPublicIpOnLaunch`, `State` |
| `#RouteTable` | `aws.ec2.route_table` | `RouteTableId` | `VpcId`, `Routes`, `Associations` |
| `#SecurityGroup` | `aws.ec2.security_group` | `GroupId` | `VpcId`, `IpPermissions`, `IpPermissionsEgress` |
| `#InternetGateway` | `aws.ec2.internet_gateway` | `InternetGatewayId` | `Attachments` |
| `#NATGateway` | `aws.ec2.nat_gateway` | `NatGatewayId` | `SubnetId`, `VpcId`, `State`, `ConnectivityType` |
| `#NetworkACL` | `aws.ec2.network_acl` | `NetworkAclId` | `VpcId`, `Entries`, `IsDefault` |
| `#VPCPeering` | `aws.ec2.vpc_peering` | `VpcPeeringConnectionId` | `RequesterVpcInfo`, `AccepterVpcInfo`, `Status` |
| `#VPCEndpoint` | `aws.ec2.vpc_endpoint` | `VpcEndpointId` | `ServiceName`, `VpcId`, `VpcEndpointType`, `State` |
| `#ElasticIP` | `aws.ec2.elastic_ip` | `AllocationId` | `PublicIp`, `AssociationId`, `Domain`, `InstanceId`, `NetworkInterfaceId` |

## Supporting Types

- `#Tag` — Standard AWS key-value tag
- `#IpPermission` — Security group rule with IP ranges, groups, prefix lists
- `#Route` — Route table entry with destination/target
- `#RouteTableAssociation` — Subnet/gateway association
- `#NetworkACLEntry` — NACL rule with port range, protocol, action

## Files Changed

- `internal/importer/bootstrap/pudl/aws/aws.cue` — New schema file
- `internal/importer/bootstrap/pudl/catalog/catalog.cue` — 11 catalog entries
- `internal/importer/cue_schemas.go` — Bootstrap check for aws package
- `internal/init/init.go` — Fixed CUE language version (v0.14.0 → v0.12.0)

## Verified

- `pudl schema list` shows all 16 types in `pudl/aws`
- Inference correctly matches mock VPC, SecurityGroup, and Subnet JSON
- `pudl list` shows imported entries with correct schema assignments
