## Local Foundation Overlays

This overlay contain all the dependencies (database, monitoring) for local development.

By design, it is decoupled from local overlay because the main purpose is to have the micros services and backend detail decouple.

### Components

All the components are deployed in the `monitoring` namespace.

| Component  | Purpose                                                        | Local access (tilt)      |
|------------|----------------------------------------------------------------|--------------------------|
| Grafana    | Dashboards and exploration (anonymous admin)                   | http://localhost:8090    |
| Alloy      | Collects pods logs (Loki), OTLP traces (Tempo), profiles (Pyroscope) | http://localhost:8091 (UI) |
| Loki       | Logs (single binary, filesystem storage)                       |                          |
| Tempo      | Traces, receives OTLP on `alloy.monitoring:4317` through Alloy |                          |
| Prometheus | Metrics, scrapes the pods annotated with `prometheus.io.scrape`|                          |
| Pyroscope  | Profiles, Alloy scrapes the pods annotated with `pyroscope.io/scrape` |                   |
| Mimir      | Long term metrics storage (deactivated)                        |                          |

Services send their traces to `alloy.monitoring.svc.cluster.local:4317` (`OTL_ENDPOINT`).

### Helm charts

The helm charts are vendored in each component `charts` folder with [vendir](https://carvel.dev/vendir/)
(see `vendir.yml`) and rendered by kustomize (`kubectl kustomize --enable-helm`).

To update a chart, bump its version in `vendir.yml` and run `vendir sync` in the component folder.
