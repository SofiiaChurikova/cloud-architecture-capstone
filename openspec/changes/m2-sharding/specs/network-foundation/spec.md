# Spec Delta

## MODIFIED Requirements

### Requirement: Private subnets are internal-only
Private subnets SHALL have no route to the internet, only the local route and a route to S3 through a gateway endpoint, and SHALL NOT assign public IPv4 addresses by default.

#### Scenario: No internet route
- **WHEN** the private route table is inspected
- **THEN** it contains the local route and the S3 endpoint route, and no route to an Internet Gateway

#### Scenario: No public IPs
- **WHEN** an instance is launched in a private subnet with default settings
- **THEN** it receives no public IPv4 address
