# Tiltfile configure and manage deployment to a local kubernetes cluster.
# This is intended to be used for developer to have a replicable production-ish setup on a laptop.

# Allowing local k8s context.
# The purpose is to enable tilt to work on a local k8s for the developers.
allow_k8s_contexts([
    'docker-desktop',
    'rancher-desktop'
])


# Overriding the default kustomize function to allow
# passing enable helm options.
# Default builtin kustomize do not allow passing options.
def kustomize(path):
    cmd = "kustomize build --enable-helm " + path
    return local(cmd, command_bat=cmd, quiet=True)

# Load local foundation dependencies.
k8s_yaml(kustomize('overlays/local-foundation'))

# LGTM monitoring stack labeled to monitoring
k8s_resource('loki', labels=["monitoring"])
k8s_resource('loki-gateway', labels=["monitoring"])
k8s_resource('grafana', port_forwards='8090:3000', labels=["monitoring"])
k8s_resource('tempo', labels=["monitoring"])
k8s_resource('tempo', labels=["monitoring"])
k8s_resource('mimir-query-frontend', labels=["monitoring"])
k8s_resource('mimir-index-cache', labels=["monitoring"])
k8s_resource('mimir-metadata-cache', labels=["monitoring"])
k8s_resource('mimir-querier', labels=["monitoring"])
k8s_resource('mimir-query-scheduler', labels=["monitoring"])
k8s_resource('mimir-chunks-cache', labels=["monitoring"])
k8s_resource('mimir-ruler', labels=["monitoring"])
k8s_resource('mimir-compactor', labels=["monitoring"])
k8s_resource('mimir-results-cache', labels=["monitoring"])
k8s_resource('mimir-overrides-exporter', labels=["monitoring"])
k8s_resource('mimir-distributor', labels=["monitoring"])
k8s_resource('mimir-nginx', labels=["monitoring"])
k8s_resource('mimir-alertmanager', labels=["monitoring"])
k8s_resource('mimir-ingester', labels=["monitoring"])
k8s_resource('mimir-store-gateway', labels=["monitoring"])
k8s_resource('mimir-smoke-test', labels=["monitoring"])

# pyroscope for profiling
k8s_resource('pyroscope', labels=["monitoring"])



# Load microservices
# Local overlays will contain all microservices built with workspace.
# k8s_yaml(kustomize('overlays/local'))

# sample namespace
# include('./sample/sampleapp/Tiltfile')
