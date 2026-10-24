## Software Development Life Cycle

SDLC is a folder that contains all the utilities tools, and documentation.

- [documentation](documentation/README.md): the decisions and guidelines of the workspace.
- [codegen](#codegen): scaffolds a new project (service) in a namespace.
- [errgen](documentation/application-error.md#coded-errors): generates the coded errors from
  `kit/errors/errors.defs.yaml` (`make errgen`).
- [onboarding](onboarding/onboarding.sh): checks the tools required by the workspace (`make onboarding`).
- [scripts/rename.sh](scripts/rename.sh): renames the Go module of the template, to start a new repository from it
  (`make rename MODULE=github.com/acme/platform`). It replaces the module path (go.mod, imports, protos, linter
  configuration, docs), the GitHub owner and repository (CODEOWNERS, badges), and regenerates the protobuf code.

### codegen
`codegen` bootstraps a new project following the workspace conventions, the developer then builds the service
from the generated skeleton (see [sample/sampleapp](../sample/sampleapp) for a complete example).

```bash
make codegen                                              # prompts for the namespace and the project
make codegen ARGS="-namespace sample -project billing"
make codegen ARGS="-namespace sample -project billing -dry-run"
```

For a new namespace it creates `<namespace>/{namespace,service-account,kustomization}.yaml` and registers the
namespace in `overlays/local/kustomization.yaml`. For the project it creates:

| File | Description |
|---|---|
| `<namespace>/<project>/cmd/<project>/main.go` | minimal main starting the kit foundation |
| `<namespace>/<project>/kube.yaml` | deployment and service, registered in `<namespace>/kustomization.yaml` |
| `<namespace>/<project>/Tiltfile` | local development, included in the root `Tiltfile` |
| `<namespace>/<project>/README.md` | project documentation |
| `.github/workflows/<namespace>.<project>.{test,build}.yaml` | unit tests and image build |

Flags: `-dry-run` prints the files without writing them, `-force` overwrites existing project files,
`-yes` skips the confirmation. Nothing is written if a file already exists (without `-force`) or a name is invalid:
names are lowercase letters and digits, and project names are unique in the workspace.

codegen is a separate Go module (`sdlc/codegen/go.mod`) so its dependencies stay out of the services, it is run
with `go run` (no install). Its templates are in `sdlc/codegen/templates`, run `go test ./... -update` in
`sdlc/codegen/cmd/codegen` to update the golden files after changing them.
