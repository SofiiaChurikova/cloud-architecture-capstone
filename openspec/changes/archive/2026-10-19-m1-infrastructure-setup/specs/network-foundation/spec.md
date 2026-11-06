# Spec Delta

## Purpose

Provides the isolated network for all components and exports its identifiers so that later stacks can use them.

## ADDED Requirements

### Requirement: VPC spans two Availability Zones
The network SHALL consist of one VPC with subnets in two Availability Zones of the region.

#### Scenario: Subnets are in different AZs
- **WHEN** the network stack is deployed
- **THEN** there is one public and one private subnet in each of two Availability Zones

### Requirement: Public subnets reach the internet
Public subnets SHALL have a route 0.0.0.0/0 to the Internet Gateway and SHALL assign public IPv4 addresses to instances by default.

#### Scenario: Public route exists
- **WHEN** the public route table is inspected
- **THEN** it contains a 0.0.0.0/0 route to the Internet Gateway

#### Scenario: Public IPs are assigned
- **WHEN** an instance is launched in a public subnet with default settings
- **THEN** it receives a public IPv4 address

### Requirement: Private subnets are internal-only
Private subnets SHALL have no route outside the VPC and SHALL NOT assign public IPv4 addresses by default.

#### Scenario: No internet route
- **WHEN** the private route table is inspected
- **THEN** it contains only the local route

#### Scenario: No public IPs
- **WHEN** an instance is launched in a private subnet with default settings
- **THEN** it receives no public IPv4 address

### Requirement: Network identifiers are exported
The network stack SHALL export the VPC id and subnet ids.

#### Scenario: Another stack uses the network
- **WHEN** another stack in the same region imports the VPC id
- **THEN** it gets the id of the VPC created by the network stack
