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
k8s_resource('grafana-agent', port_forwards='8091:80', labels=["monitoring"])
k8s_resource('prometheus', labels=["monitoring"])
k8s_resource('tempo', labels=["monitoring"])

# pyroscope for profiling
k8s_resource('pyroscope', labels=["monitoring"])


# Load microservices
# Local overlays will contain all microservices built with workspace.
k8s_yaml(kustomize('overlays/local'))

# sample namespace
include('./sample/sampleapp/Tiltfile')
