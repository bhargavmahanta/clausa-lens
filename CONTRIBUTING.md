# Contributing to CausaLens

Thanks for helping improve CausaLens. The project favors small, evidence-backed changes that preserve the deterministic judge workflow.

## Development flow

1. Create a focused branch from `team/integration`.
2. Keep contract changes explicit. [`docs/CONTRACTS.md`](docs/CONTRACTS.md) is authoritative for public field names, enums, APIs, and lifecycle states.
3. Add or update tests for changed behavior.
4. Run the relevant verification commands below.
5. Open a pull request into `team/integration` using the repository template.

Do not open feature pull requests directly into `main`; `main` is the stable demo baseline.

## Verification

### Go

```bash
go test -count=1 ./...
go test -race -count=1 ./...
go vet ./...
gofmt -l internal/replay cmd/core-api
```

### Command Center

```bash
cd web
npm ci
npm run lint
npm run typecheck
npm test
npm run build
```

For UI changes, verify the critical workflow at desktop and mobile sizes and inspect console/network failures.

## Pull request expectations

- Explain the user-visible behavior and why the change is needed.
- Keep unrelated refactors out of focused changes.
- Include commands and outcomes used for verification.
- Call out security, schema, migration, compatibility, or deployment impact.
- Never commit real credentials, production data, or unsanitized fixtures.

## Commit style

Use concise conventional-style subjects where practical:

```text
feat(replay): add approved latency intervention
fix(web): prevent mobile navigation overlap
docs: clarify isolated replay boundary
```
