# Security Policy

## Supported Versions

We actively maintain and provide security updates for the following versions of **gohcms**:

| Version | Supported          | Notes                              |
| ------- | ------------------ | ---------------------------------- |
| v0.3.x  | :white_check_mark: | Current active release line        |
| < v0.3  | :x:                | Legacy / pre-release versions      |

---

## Reporting a Vulnerability

The gohcms team takes security vulnerabilities seriously. If you discover a security issue or potential vulnerability in this project, please follow the procedure below to report it responsibly.

### 1. How to Report

Please **DO NOT** create a public GitHub issue for security-sensitive bugs or exploits.

Instead, report vulnerabilities through one of the following channels:

- **GitHub Private Vulnerability Reporting (Recommended)**:  
  Navigate to the **Security** tab of the repository on GitHub and click **"Report a vulnerability"** to submit an advisory draft directly and confidentially to the maintainers.
- **Maintainer Contact**:  
  If private vulnerability reporting is unavailable, reach out directly to the repository maintainer (`yutapok`) via GitHub profile contact.

### 2. What to Include in Your Report

To help us investigate and resolve the issue quickly, please provide as much detail as possible:

- **Type of vulnerability** (e.g., XSS, CSRF, Authentication Bypass, Path Traversal, Resource Exhaustion).
- **Affected versions / components** (e.g., CLI `--role=admin`, `/media` endpoint, REST API).
- **Step-by-step reproduction instructions** or a minimal Proof of Concept (PoC).
- **Potential impact** and exploitation scenarios.
- Any suggested mitigations or patches (if available).

### 3. Response Process & Timeline

- **Acknowledgment**: We aim to acknowledge receipt of your vulnerability report within 48 hours.
- **Investigation & Triage**: We will investigate the reported issue, assess the severity, and keep you informed of our findings.
- **Resolution & Release**: Once a fix is prepared and verified, we will release a security patch update and publish a GitHub Security Advisory acknowledging your contribution (unless you request to remain anonymous).

---

## Security Best Practices for Deployments

When deploying **gohcms** in production or publicly accessible environments:

1. **Role Separation**: Always run with `--role=api` for public-facing Headless REST APIs, keeping the Admin UI (`--role=admin`) behind an internal VPN, Tailscale, or authenticated reverse proxy.
2. **Mandatory Authentication**: Always configure `CMS_ADMIN_USER` and `CMS_ADMIN_PASSWORD` (or CLI flags) when exposing the Admin UI. Never bind to `0.0.0.0` without credentials or use `--allow-unauthenticated` in production.
3. **API Key Scopes**: Use scoped API keys (Read-only vs. Read-Write) rather than sharing full-access tokens across client applications.
