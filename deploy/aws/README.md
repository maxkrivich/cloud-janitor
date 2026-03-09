# AWS IAM Setup for Cloud Janitor

This directory contains IAM policy templates for setting up Cloud Janitor in your AWS accounts.

## Files

- `cloud-janitor-policy.json` - IAM policy with permissions for EC2, EBS, Snapshots, and Elastic IPs
- `cloud-janitor-trust-policy.json` - Trust policy template for the IAM role

## Setup Instructions

### 1. Create the IAM Role

Replace `ACCOUNT_ID` and `YOUR_IAM_USER` in the trust policy with your values, then create the role:

```bash
# Create the role with the trust policy
aws iam create-role \
  --role-name CloudJanitorRole \
  --assume-role-policy-document file://deploy/aws/cloud-janitor-trust-policy.json

# Attach the permissions policy
aws iam put-role-policy \
  --role-name CloudJanitorRole \
  --policy-name CloudJanitorPolicy \
  --policy-document file://deploy/aws/cloud-janitor-policy.json
```

### 2. Configure Cloud Janitor

Update your `cloud-janitor.yaml` with the role ARN:

```yaml
aws:
  accounts:
    - id: "123456789012"
      name: "dev-account"
      role: "arn:aws:iam::123456789012:role/CloudJanitorRole"
  regions:
    - us-east-1
    - us-west-2
```

### 3. Test the Configuration

```bash
# Verify role assumption works
aws sts assume-role \
  --role-arn arn:aws:iam::123456789012:role/CloudJanitorRole \
  --role-session-name test-session

# Run Cloud Janitor in dry-run mode
cloud-janitor list --dry-run
```

## Multi-Account Setup

For multiple accounts, create the CloudJanitorRole in each target account with the same trust policy pointing to your central IAM user or role.

## Least Privilege

The provided policy grants full access to the supported resource types. For production use, consider:

1. Adding resource-level restrictions using `Condition` blocks
2. Limiting to specific regions using `aws:RequestedRegion`
3. Requiring specific tags using `aws:ResourceTag`

Example with region restriction:

```json
{
  "Sid": "EC2DeletePermissions",
  "Effect": "Allow",
  "Action": [
    "ec2:TerminateInstances",
    "ec2:DeleteVolume"
  ],
  "Resource": "*",
  "Condition": {
    "StringEquals": {
      "aws:RequestedRegion": ["us-east-1", "us-west-2"]
    }
  }
}
```
