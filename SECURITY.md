# Security Policy

## Supported Versions

Only the latest stable release receives security updates.

## Reporting a Vulnerability

Please **do not open a public GitHub Issue** for security vulnerabilities.

Instead, contact the maintainers privately.

Include:

- Description
- Impact
- Reproduction
- Proof of Concept (if available)

We will investigate the report as quickly as possible.

## Automated Security Monitoring

This repository uses:

- CodeQL analysis for static security findings
- govulncheck for Go vulnerability detection
- Gitleaks for secret scanning in Git history
- Dependabot for dependency update PRs

Maintainers should also enable GitHub secret scanning and push protection in repository settings when available, and require security checks in branch protection rules.