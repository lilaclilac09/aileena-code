# Acceptance audit — 2026-11-22

Scope: existing work at `origin/main` `1185fdac843a376b3ef64b15355c010f7c579edf`; all merged PRs returned by the paginated GitHub API; tracked docs, Go source/tests and repository instructions. Only this repository's [ACCEPTANCE.md](../ACCEPTANCE.md) and the instructions it incorporates apply. Historical work is assessed against today's standard, not assumed to have been governed by it at the time.

| # | Item | Fails rule | Source rule | Fix or not fixable |
|---|------|------------|-------------|--------------------|
| 1 | README had no single product-status/next-action line; its related hosted demo could not establish Go CLI production verification | "Demo ≠ product"; "LIVE / BLOCKED / LOCAL ONLY + next human action in one line"; "if you cannot prove it on production, do not claim it is fixed" | `ACCEPTANCE.md` §5, quoting `ship-visible-mvp.mdc` / `AGENTS.md` | Added LOCAL ONLY, offline scope and next human action. Kept the existing candid Demo disclosure and distinguished the separate Machina implementation. Homepage is null, Pages API returns HTTP 404 and deployments API returns zero entries. Related `https://www.aileena.xyz/` returns HTTP 200; `/api/mcp` returns HTTP 405 to GET. Neither proves this Go binary was run against a live model. |

## Verification

- Exactly one merged PR: [#1](https://github.com/lilaclilac09/aileena-code/pull/1), process/acceptance files only. Its current body passes `python3 scripts/check_acceptance.py`; merge SHA is an ancestor of `origin/main`. No merged UI PRs and no browser UI exist in this repository. The separate Machina site is not this repository's UI and was not audited or changed.
- Read `AGENTS.md`, `CLAUDE.md`, `.cursor/rules/acceptance.mdc` and README. No extra English-only rule or okc/job-ID requirement exists. README says "never commit this" for the API key and "Run it in a scratch directory or a throwaway checkout" because there is no sandbox. Tracked-file/source review found no committed credentials; test credentials are mock values. No real model call or file-editing agent session was launched in the source checkout.
- README's existing Demo section already discloses that the Go binary had not been run against a live model, and labels the displayed diff/output as a separate TypeScript/Machina example. The embedded diff and command output are artifacts; they are not treated as Go live-model proof. No historical demonstration was reproduced or silently promoted to current verification.
- `main.go` is 396 lines; `go.mod` has no third-party dependencies. The five tutorial-step commits listed by README exist in the full git history. The deliberately documented no-sandbox/replace-all/no-step-cap behavior was not redesigned.
- `go vet ./...`: passed. `go test ./...`: passed, including the local mock model/tool loop, edit create/replace/error cases and directory-list filtering. `go build -o /home/ubuntu/accept-kit/audit/aileena-code/aileena-code .`: passed, placing the binary outside the source tree.
- Prepared PR body passes this repo's checker; `git diff --check` passes. No production inference or model-provider interoperability claim is made by these offline checks.
- Forbidden-claim search found no unqualified completion phrases outside acceptance-rule prohibitions/source quotations. There is no results ledger requiring a separate proof-counting change.

Passed: merged PR #1's current closer and merge ancestry; offline Go tests, vet and build; explicit live-model limitation; tutorial history and small/zero-dependency source claims; no tracked credentials found. Browser screenshots/interaction proof are not applicable to this CLI-only repository.

Observed, no rule covers it: the tutorial intentionally has no sandbox, retry policy, context management or step cap, and uses replace-all edits. Its README explicitly documents these tradeoffs. No taste-based hardening, redesign, dependency or cross-repository UI work was added.

Status: LOCAL ONLY — next human action: run the binary in a scratch directory with your own API key to verify a real model interaction; this audit does not claim that run occurred.
