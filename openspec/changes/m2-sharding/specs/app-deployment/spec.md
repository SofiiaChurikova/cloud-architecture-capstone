# Spec Delta

## Purpose

The app runs in AWS from code and is deployed by a pipeline.

## ADDED Requirements

### Requirement: App is described in CloudFormation
The coordinator and the shards SHALL be created by a CloudFormation stack that uses the network from M1.

#### Scenario: Deploy the app stack
- **WHEN** the app stack is deployed
- **THEN** 1 coordinator and 2 shards are running in the M1 VPC

### Requirement: Pipeline deploys infra and app
A pipeline SHALL build the code and deploy the infra and the app.

#### Scenario: Push to main
- **WHEN** a commit is pushed to `main`
- **THEN** the pipeline builds the code and deploys it
