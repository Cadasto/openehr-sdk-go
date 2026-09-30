## Summary

<!-- What changed and why, in prose. Name the decisions taken and the alternatives not taken.
     No commit list, no task ticks, no finding ids: findings live in the findings file and the
     review threads (docs/ai-workflow.md § Review). -->

## Checklist

- [ ] Scoped to one logical change ([CONTRIBUTING.md](https://github.com/Cadasto/openehr-sdk-go/blob/main/CONTRIBUTING.md))
- [ ] `make ci` passes locally
- [ ] Consumer-visible change called out in this PR so it can be folded in (maintainers curate [`CHANGELOG.md`](https://github.com/Cadasto/openehr-sdk-go/blob/main/CHANGELOG.md) on release; see [AGENTS.md](https://github.com/Cadasto/openehr-sdk-go/blob/main/AGENTS.md#code-style-and-conventions))
- [ ] REQ / PROBE behaviour updated in [`docs/specifications/traceability.yaml`](https://github.com/Cadasto/openehr-sdk-go/blob/main/docs/specifications/traceability.yaml) when applicable (`make spec-check`)
- [ ] Commit messages use [Conventional Commits](https://www.conventionalcommits.org/); REQ-NNN / PROBE-NNN cited where relevant

## Spec / probes

<!-- REQ-NNN, PROBE-NNN, ADR, plan link — or, for a change that alters no normative statement:
     "Lane: maintenance (no normative change)" (docs/development-process.md § Two lanes). -->

## Verification

<!-- `make ci`: what the output said, or "n/a" for docs-only.
     Red before green: the tests that failed before the change.
     Can-fail proof: the guards removed and the tests that then failed. -->

## Notes for review

<!-- Where to look hardest; what is out of scope; a known gap left on purpose. Omit when empty. -->
