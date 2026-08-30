# Security Policy

## Reporting a vulnerability

Please do not disclose security vulnerabilities in public issues or discussions.

Use GitHub's **Report a vulnerability** flow under the repository's Security tab. Include:

- affected component and version/commit;
- reproduction steps or proof of concept;
- expected impact;
- any known mitigations; and
- whether real credentials or sensitive data may be involved.

If private vulnerability reporting is unavailable, contact a repository maintainer privately before sharing details.

## Scope

Security-sensitive areas include:

- replay isolation and network boundaries;
- fixture extraction and sanitization;
- capsule integrity and validation;
- database credentials and migrations;
- event ingestion and untrusted evidence;
- API proxying and external service adapters; and
- any path that could reach production systems during replay.

The credentials in `deploy/compose.yaml` are local demo defaults only. Never reuse them outside the disposable local environment or commit real credentials to the repository.

## Supported versions

This is an active hackathon MVP. Security fixes are applied to the current `main` and `team/integration` branches; older commits and branches are not maintained.
