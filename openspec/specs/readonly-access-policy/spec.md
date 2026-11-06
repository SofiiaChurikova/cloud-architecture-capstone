# readonly-access-policy Specification

## Purpose
Describes in code an IAM policy and role that give view-only access to the project's AWS resources, with no ability to change them.

## Requirements

### Requirement: Only view actions are permitted
The policy SHALL permit only actions that read resource metadata, such as Describe, List and Get calls.

#### Scenario: Viewing a resource
- **WHEN** the policy is checked against `ec2:DescribeVpcs`
- **THEN** the action is allowed

### Requirement: No changes to resources
The policy SHALL NOT permit any action that creates, updates or deletes a resource.

#### Scenario: Attempt to change a resource
- **WHEN** the policy is checked against `ec2:CreateVpc` or `s3:PutObject`
- **THEN** the action is not allowed

### Requirement: Only a trusted principal can assume the role
The role SHALL trust exactly one principal, defined in its trust policy, and reject all others.

#### Scenario: Untrusted principal tries to assume the role
- **WHEN** a principal that is not listed in the trust policy calls `sts:AssumeRole`
- **THEN** access is denied

### Requirement: Template is checked as code
Since the sandbox does not allow creating IAM resources, the template SHALL be validated as code, and the sandbox refusal SHALL be recorded.

#### Scenario: Validation without deployment
- **WHEN** the template is checked with `aws cloudformation validate-template`
- **THEN** no errors are reported

#### Scenario: Deployment is refused by the sandbox
- **WHEN** the template is deployed in the AWS Academy sandbox
- **THEN** creation fails with an access denied error, and the error is saved as evidence
