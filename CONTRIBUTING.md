# Contributing

Welcome to Workspace!

Please feel free to open an issue or participate in the project via Pull Request.

Any contribution is warmly welcome.

## Development

- `make onboarding` checks the required tools, `make help` lists the development commands.
- `make lint test` must pass, `make test-integration` runs the kit tests with their services (docker compose).
- Regenerate the code you changed the source of: `make proto-gen` (protos), `make errgen` (`kit/errors/errors.defs.yaml`),
  `make tidy` (dependencies, the vendor directory is committed).
- The guidelines are in [sdlc/documentation](sdlc/documentation/README.md).
