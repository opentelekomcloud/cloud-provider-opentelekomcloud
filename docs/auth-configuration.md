# Authentication Configuration

The cloud-provider-opentelekomcloud supports two authentication methods for communicating with OTC APIs: **Password** and **AK/SK** (Access Key / Secret Key).

Configuration is provided via an INI file (typically `/etc/kubernetes/cloud.conf`). Environment variables are used as fallbacks when a field is not set in the config file.

## Password Authentication

```ini
[Global]
auth-url=https://iam.eu-de.otc.t-systems.com/v3
username=my-user
password=my-password
domain-name=my-domain
tenant-name=eu-de_project
region=eu-de
```

## AK/SK Authentication

```ini
[Global]
auth-url=https://iam.eu-de.otc.t-systems.com/v3
access-key=AKIA...
secret-key=wJal...
project-id=abc123
region=eu-de
domain-name=my-domain
```

### Temporary Credentials

AK/SK supports temporary credentials with a security token:

```ini
[Global]
auth-url=https://iam.eu-de.otc.t-systems.com/v3
access-key=AKIA...
secret-key=wJal...
security-token=FwoGZXIv...
project-id=abc123
region=eu-de
domain-name=my-domain
```

## TLS Configuration

```ini
[Global]
# ... auth fields ...
ca-file=/etc/ssl/certs/custom-ca.pem
tls-insecure=false
```

## Environment Variables

The following environment variables are used as fallbacks:

| Field           | Environment Variable |
|-----------------|---------------------|
| auth-url        | `OS_AUTH_URL`       |
| username        | `OS_USERNAME`       |
| password        | `OS_PASSWORD`       |
| access-key      | `OS_ACCESS_KEY`     |
| secret-key      | `OS_SECRET_KEY`     |
| security-token  | `OS_SECURITY_TOKEN` |
| domain-name     | `OS_DOMAIN_NAME`    |
| domain-id       | `OS_DOMAIN_ID`      |
| tenant-id       | `OS_TENANT_ID`      |
| tenant-name     | `OS_TENANT_NAME`    |
| project-id      | `OS_PROJECT_ID`     |
| region          | `OS_REGION_NAME`    |

## Auth Method Priority

When both password and AK/SK credentials are provided, **AK/SK takes priority**.

## Kubernetes Deployment

Pass the config file to the cloud-controller-manager:

```bash
cloud-controller-manager \
  --cloud-provider=opentelekomcloud \
  --cloud-config=/etc/kubernetes/cloud.conf
```
