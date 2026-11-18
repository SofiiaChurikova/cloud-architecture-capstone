# Spec Delta

## Purpose

The replicated store runs in AWS from code.

## ADDED Requirements

### Requirement: Seven nodes in the app stack
The app stack SHALL create 1 coordinator and 2 shards x 3 replicas, 7 EC2 instances, under the sandbox limit of 9.

#### Scenario: Deploy the app stack
- **WHEN** the app stack is deployed
- **THEN** 7 instances are running and `GET /internal/shards` shows 2 shards with 3 replicas each

### Requirement: Replicas are in both AZs
The replicas of every shard SHALL be in both private subnets, so one shard is not in one AZ only.

#### Scenario: Replica placement
- **WHEN** the instances of `shard-1` are listed
- **THEN** they are in `private-a` and `private-b`

### Requirement: Followers reach the leader
Shard nodes SHALL accept port 8081 from the coordinator and from other shard nodes, and from nothing else.

#### Scenario: Follower pulls the log
- **WHEN** a follower asks its leader for log entries
- **THEN** the security group lets the request through
