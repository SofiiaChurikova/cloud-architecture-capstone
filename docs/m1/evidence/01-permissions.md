# Task 1 - sandbox permissions (role voclabs)

## $ aws sts get-caller-identity

```
{
    "UserId": "AROAXHDADK2HDA2T2DNEM:user5166800=schurikova@kse.org.ua",
    "Account": "496270268046",
    "Arn": "arn:aws:sts::496270268046:assumed-role/voclabs/user5166800=schurikova@kse.org.ua"
}
```

## $ aws iam list-roles --query 'Roles[].RoleName' --output text

```
AWSServiceRoleForAPIGateway	AWSServiceRoleForAutoScaling	AWSServiceRoleForAWSCloud9	AWSServiceRoleForCloudWatchEvents	AWSServiceRoleForElastiCache	AWSServiceRoleForElasticBeanstalk	AWSServiceRoleForElasticLoadBalancing	AWSServiceRoleForOrganizations	AWSServiceRoleForRDS	AWSServiceRoleForResourceExplorer	AWSServiceRoleForSupport	AWSServiceRoleForTrustedAdvisor	EMR_AutoScaling_DefaultRole	EMR_DefaultRole	EMR_EC2_DefaultRole	lambda-run-role	myBasicLambdaRole	myS3Role	vocareum	vocareum-eventbridge	voclabs
```

## $ aws iam list-attached-role-policies --role-name voclabs

```
{
    "AttachedPolicies": [
        {
            "PolicyName": "ReadOnlyAccess",
            "PolicyArn": "arn:aws:iam::aws:policy/ReadOnlyAccess"
        }
    ]
}
```

## $ aws iam list-roles --max-items 1

```
{
    "Roles": [
        {
            "Path": "/aws-service-role/ops.apigateway.amazonaws.com/",
            "RoleName": "AWSServiceRoleForAPIGateway",
            "RoleId": "AROAXHDADK2HFN4AKGF6H",
            "Arn": "arn:aws:iam::496270268046:role/aws-service-role/ops.apigateway.amazonaws.com/AWSServiceRoleForAPIGateway",
            "CreateDate": "2024-10-21T13:51:14+00:00",
            "AssumeRolePolicyDocument": {
                "Version": "2012-10-17",
                "Statement": [
                    {
                        "Effect": "Allow",
                        "Principal": {
                            "Service": "ops.apigateway.amazonaws.com"
                        },
                        "Action": "sts:AssumeRole"
                    }
                ]
            },
            "Description": "The Service Linked Role is used by Amazon API Gateway.",
            "MaxSessionDuration": 3600
        }
    ],
    "NextToken": "eyJNYXJrZXIiOiBudWxsLCAiYm90b190cnVuY2F0ZV9hbW91bnQiOiAxfQ=="
}
```

## $ aws iam create-user --user-name test-user

```
An error occurred (AccessDenied) when calling the CreateUser operation: User: arn:aws:sts::496270268046:assumed-role/voclabs/user5166800=schurikova@kse.org.ua is not authorized to perform: iam:CreateUser on resource: arn:aws:iam::496270268046:user/test-user because no identity-based policy allows the iam:CreateUser action
```

## $ aws iam simulate-principal-policy --policy-source-arn arn:aws:iam::496270268046:role/voclabs --action-names ec2:CreateVpc iam:CreateRole

```
{
    "EvaluationResults": [
        {
            "EvalActionName": "ec2:CreateVpc",
            "EvalResourceName": "*",
            "EvalDecision": "allowed",
            "MatchedStatements": [
                {
                    "SourcePolicyId": "role_voclabs_Pvoclabs",
                    "SourcePolicyType": "IAM Policy",
                    "StartPosition": {
                        "Line": 1,
                        "Column": 38
                    },
                    "EndPosition": {
                        "Line": 1,
                        "Column": 1810
                    }
                }
            ],
            "MissingContextValues": [],
            "OrganizationsDecisionDetail": {
                "AllowedByOrganizations": true
            }
        },
        {
            "EvalActionName": "iam:CreateRole",
            "EvalResourceName": "*",
            "EvalDecision": "implicitDeny",
            "MatchedStatements": [],
            "MissingContextValues": [
                "ec2:PlacementGroupStrategy",
                "rds:DatabaseClass",
                "rds:StorageSize",
                "ec2:VolumeType",
                "rds:Piops",
                "ec2:InstanceType",
                "ec2:VolumeSize",
                "rds:Databaseclass",
                "ec2:Owner",
                "ec2:Tenancy",
                "autoscaling:InstanceType",
                "rds:DatabaseEngine",
                "cloud9:InstanceType",
                "s3express:SessionMode"
            ],
            "OrganizationsDecisionDetail": {
                "AllowedByOrganizations": true
            }
        }
    ]
}
```

## $ aws ec2 create-vpc --cidr-block 10.99.0.0/16 --dry-run

```
An error occurred (DryRunOperation) when calling the CreateVpc operation: Request would have succeeded, but DryRun flag is set.
```

## $ aws ec2 run-instances --dry-run --instance-type t3.micro --image-id <al2023>

```
An error occurred (DryRunOperation) when calling the RunInstances operation: Request would have succeeded, but DryRun flag is set.
```

## $ aws ec2 run-instances --dry-run --instance-type t3.large --image-id <al2023>

```
An error occurred (UnauthorizedOperation) when calling the RunInstances operation: You are not authorized to perform this operation. User: arn:aws:sts::496270268046:assumed-role/voclabs/user5166800=schurikova@kse.org.ua is not authorized to perform: ec2:RunInstances on resource: arn:aws:ec2:us-east-1:496270268046:instance/* with an explicit deny in an identity-based policy. Encoded authorization failure message: kkA_L0GmY3lLkXzbqIKbdJ2Lekpb-2krDifhVWuPK1-29H_hE1iAk7KOe0tJMI632yXPIOWF9TRzG91jQDpJqOXvfLv_LLWOmpInzvnc4M6y5RI6GXzaGRXu9_LtBbpUZy0-i5_blIqNcPBXYNpP6MV-JDRJHKygerEVG6bWuWYdjOP84wAsKEewF2KtLCEm5IdqRfxGga8poyk2aWA689Gu8gNEFMd5iWuHgB73pDsJKm5xWfoy2kuTRYiiBsca-xSfgjGySoWz0ulqEWSCSbDB8vx_2lXmn4w5xk6NQPVgR0BLDpWUTGpO6tMv6Cc0VgzoiGyNxFmadPmhSZXiVbDTfUFRmVbCdyMBj8k4lyhfIZ7e71PEbeqV20lnXOAmF8ne1hX4jIYgZlPfvLRzusVGL7BYX0jBc3UO4mEEAG5oPJaqasCzf4pb3KOXAeeh7U9NZokhZMvM_c1UV4ICQKf_V7tzTPZkBZbblPXmhqfOrjUsIzedY7WPoTWga0D9GDvPp9oXMTGp2RSjfXE2Apa2GJ8hYboR1QpYH-ael8IHIl55tDLsLVaFGI3_brSQcwS_3GZNW8z0mQIr2WUlt12-qWLmLhWB4orZWbF43UCg_7a-2OOP6-YRfVMfIDRZqDzoSVw9MGx3f8_XSgmJ1LB8u4czZtE380SG8yolX0kDTDApnMqFBxMorR3gS1I3cjbTqlKFy0UQdR6Hd4Ayz1ZakK9vAEiAc8ocmzgrbE2zHwfyljVUkRfB4O_XAgPyxOnHojmpjE5bUtBHYGZOoZ_mSBLUIvSkJ5E22kr7BeL83NoNvvNGPtxSCI8X57599WyZ8HFe7eib7_QJijugD7TYs-7QzOYVpEcFuaoKr6tT-8jjHS6iQUHcEBQZXcawQmecZRORq3tbFUOgM_i6DT1XS-Wkl-kx9KyxVgSyClkTvtvTNWY8kT4UQJt5owbJ4sCJpVcgiLeORf_AMmhjUBaIwZq7I5G2zxrM6XSPlbHBww
```

## $ aws ec2 describe-vpcs --region eu-central-1

```
An error occurred (UnauthorizedOperation) when calling the DescribeVpcs operation: You are not authorized to perform this operation. User: arn:aws:sts::496270268046:assumed-role/voclabs/user5166800=schurikova@kse.org.ua is not authorized to perform: ec2:DescribeVpcs with an explicit deny in a service control policy: arn:aws:organizations::111832637897:policy/o-5oy5z3q3ij/service_control_policy/p-j269txxv
```

