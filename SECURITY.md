# Security Policy

## Supported Versions

| Version | Supported          |
| ------- | ------------------ |
| latest  | :white_check_mark: |
| < 1.0   | :x:                |

## Reporting a Vulnerability

We take the security of Cloud Janitor seriously. If you believe you have found a security vulnerability, please report it to us as described below.

### How to Report

**Please do not report security vulnerabilities through public GitHub issues.**

Instead, please report them via email to: **security@example.com** (replace with your actual security email)

Alternatively, you can use GitHub's private vulnerability reporting feature:
1. Go to the [Security tab](https://github.com/maxkrivich/cloud-janitor/security) of this repository
2. Click "Report a vulnerability"
3. Fill out the form with details about the vulnerability

### What to Include

Please include the following information in your report:

- Type of vulnerability (e.g., credential exposure, injection, etc.)
- Full paths of source file(s) related to the vulnerability
- Location of the affected source code (tag/branch/commit or direct URL)
- Step-by-step instructions to reproduce the issue
- Proof-of-concept or exploit code (if possible)
- Impact of the issue, including how an attacker might exploit it

### Response Timeline

- **Initial Response**: Within 48 hours, we will acknowledge receipt of your report
- **Status Update**: Within 7 days, we will provide an initial assessment
- **Resolution**: We aim to resolve critical vulnerabilities within 30 days

### Disclosure Policy

- We will work with you to understand and resolve the issue quickly
- We will keep you informed about our progress
- We will credit you in the security advisory (unless you prefer to remain anonymous)
- We ask that you give us reasonable time to address the issue before any public disclosure

## Security Best Practices for Users

When using Cloud Janitor, please follow these security best practices:

### AWS Credentials

1. **Never hardcode credentials** - Use IAM roles, environment variables, or AWS credential files
2. **Use least privilege** - Grant only the permissions necessary for scanning
3. **Use temporary credentials** - Prefer IAM roles with temporary credentials over long-lived access keys
4. **Rotate credentials regularly** - If using access keys, rotate them periodically

### Recommended IAM Policy

Use a read-only policy for scanning. Here's a minimal example:

```json
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Effect": "Allow",
      "Action": [
        "ec2:Describe*",
        "elasticloadbalancing:Describe*",
        "rds:Describe*",
        "lambda:List*",
        "lambda:Get*",
        "s3:ListAllMyBuckets",
        "s3:GetBucketLocation",
        "cloudwatch:GetMetricStatistics"
      ],
      "Resource": "*"
    }
  ]
}
```

### Running in CI/CD

1. Use OIDC authentication when possible (e.g., GitHub Actions OIDC with AWS)
2. Never store credentials in code or configuration files
3. Use secrets management for any required credentials
4. Run with minimal permissions needed for the scan

## Known Security Considerations

- Cloud Janitor requires read access to your AWS resources
- Scan results may contain resource IDs and metadata - treat output as sensitive
- When using `--output json` or `--output yaml`, ensure output files are stored securely
