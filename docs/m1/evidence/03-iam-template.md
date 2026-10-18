# Tasks 2-3 - read-only IAM policy and role

## $ aws cloudformation validate-template --template-body file://infra/iam-readonly.yaml

```
{
    "Parameters": [
        {
            "ParameterKey": "TrustedPrincipalArn",
            "NoEcho": false,
            "Description": "Who may assume the role"
        }
    ],
    "Description": "M1 read-only IAM policy and role. Cannot be deployed - delivered and validated as code only.",
    "Capabilities": [
        "CAPABILITY_NAMED_IAM"
    ],
    "CapabilitiesReason": "The following resource(s) require capabilities: [AWS::IAM::ManagedPolicy]"
}
```

## $ aws cloudformation deploy --stack-name capstone-iam --template-file infra/iam-readonly.yaml --capabilities CAPABILITY_NAMED_IAM ...

```
Waiting for changeset to be created..
Waiting for stack create/update to complete

Failed to create/update the stack. Run the following command
to fetch the list of events leading up to the failure
aws cloudformation describe-stack-events --stack-name capstone-iam
```

