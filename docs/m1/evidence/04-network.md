# Tasks 4-5 - network stack

## $ aws cloudformation deploy --stack-name capstone-network --template-file infra/network.yaml

```
Waiting for changeset to be created..
Waiting for stack create/update to complete
Successfully created/updated stack - capstone-network
```

## $ aws ec2 describe-subnets ... (name, CIDR, AZ, MapPublicIpOnLaunch)

```
-------------------------------------------------------
|                   DescribeSubnets                   |
+-----------+-----------------+--------------+--------+
|  public-a |  10.20.0.0/24   |  us-east-1a  |  True  |
|  private-a|  10.20.16.0/20  |  us-east-1a  |  False |
|  private-b|  10.20.32.0/20  |  us-east-1b  |  False |
|  public-b |  10.20.1.0/24   |  us-east-1b  |  True  |
+-----------+-----------------+--------------+--------+
```

