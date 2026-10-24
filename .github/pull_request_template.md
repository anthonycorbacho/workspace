## Description

<!-- What does this change and why? Link the issue if any. -->

## Checklist

- [ ] Tests added or updated (`make test`, `make test-integration` for the kit services)
- [ ] `make lint` and `make lint-proto` pass
- [ ] Generated code is up to date: `make proto-gen` (proto changes), `make errgen` (`kit/errors/errors.defs.yaml` changes)
- [ ] `make tidy` run if the dependencies changed (the vendor directory is committed)
- [ ] Codegen templates and golden files updated if the scaffolding conventions changed (`sdlc/codegen`)
- [ ] Documentation updated (README, `sdlc/documentation`, package docs)
