# Security Triage Guide

## Required security checks

Configure branch protection for `main` (and `master` if used) to require:

- `CodeQL / Analyze`
- `govulncheck / Scan dependencies and code`
- `Secret Scan / Gitleaks`

## Alert triage process

1. Confirm reproducibility and affected versions.
2. Assign severity:
   - **Critical**: remote code execution, auth bypass, or active exploitation.
   - **High**: significant confidentiality/integrity/availability impact.
   - **Medium**: constrained impact requiring unusual preconditions.
   - **Low**: minor impact or hard-to-exploit weakness.
3. Open or link a tracking issue/PR.
4. Apply fix and backport if needed.
5. Close alerts with rationale.

## Target response times

- **Critical**: same day
- **High**: within 3 days
- **Medium**: within 14 days
- **Low**: next normal maintenance cycle
