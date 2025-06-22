# linkerd GRPCRoute per-route metrics repro

## Prerequisites

Install the following tools:

- **Task**: [taskfile.dev](https://taskfile.dev/)
- **k3d**: [k3d.io](https://k3d.io/)
- **Linkerd CLI**: [linkerd.io/getting-started](https://linkerd.io/getting-started/)

## Setup

```bash
task setup
```

## Viewing Metrics

Port-forward to the linkerd-proxy container (port 4191) in both pods:

```bash
# Time service metrics
kubectl port-forward $(kubectl get pod -l app=time-service -o jsonpath='{.items[0].metadata.name}') 4191:4191 -c linkerd-proxy

# Time client metrics
kubectl port-forward $(kubectl get pod -l app=time-client -o jsonpath='{.items[0].metadata.name}') 4191:4192 -c linkerd-proxy
```

## Cleanup

```bash
task teardown
```
