# Security Policy

## Supported Versions

We release patches and security fixes for the current and recent major/minor versions.

| Version | Supported          |
| ------- | ------------------ |
| 1.x.x   | :white_check_mark: |
| < 1.0   | :x:                |

---

## Reporting a Vulnerability

If you believe you have discovered a security vulnerability in the **FogSense Device Runtime**, please do **not** open a public issue or discuss it in public channels.

### Preferred Method: GitHub Private Vulnerability Reporting

Please report security vulnerabilities through GitHub's [Private Vulnerability Reporting](https://github.com/aprountzos/fogsense-device-runtime/security/advisories/new) feature:
1. Navigate to the **Security** tab of the repository.
2. Click on **Advisories**.
3. Click **Report a vulnerability**.
4. Fill in the advisory form with detailed reproduction steps, potential impact, and suggested fixes if available.

### Alternative Method: Email

If you cannot use GitHub Advisories, you may email the maintainer at `aprountzos@users.noreply.github.com` with:
- A descriptive subject line (e.g. `[SECURITY] Potential vulnerability in fogsense-device-runtime`).
- A description of the issue and its potential impact.
- Step-by-step instructions or proof-of-concept code to reproduce the issue.
- Details regarding affected versions and environment.

---

## What to Expect

- **Acknowledgment**: We will acknowledge receipt of your report within **48 hours**.
- **Assessment**: We will investigate and confirm whether the reported behavior constitutes a security vulnerability.
- **Coordination & Fix**: If verified, we will work on a fix in a private branch/draft advisory and coordinate a patch release.
- **Disclosure**: Once resolved, we will publish a security advisory and credit the reporter (unless you prefer to remain anonymous).

---

## Security Considerations for Deployments

Please keep in mind the intended operating model of FogSense Device Runtime:

1. **Shared Secrets over Plain HTTP**: In default development configurations, device and control tokens are passed as HTTP authorization headers over unencrypted channels. Do not expose the device runtime or mock gateway directly to the public internet without a reverse proxy terminating TLS (HTTPS).
2. **Local and Edge Network Scopes**: The runtime is designed for edge/fog computing simulation and lab testbeds. Ensure network isolation (e.g. internal Docker networks, private subnets) between simulated devices and untrusted hosts.
3. **Spec Injection & Validation**: Ensure device specification YAML files are sourced from trusted paths, as device specifications dictate actuator execution patterns and polling routines.

