# Kubernetes Deployment Examples

Example manifests for deploying nginx-clickhouse in Kubernetes. Two deployment modes are provided:

## Sidecar Mode

Runs nginx-clickhouse alongside NGINX in the same pod, reading logs from a shared `emptyDir` volume.

```sh
kubectl apply -f configmap.yaml
kubectl apply -f secret.yaml
kubectl apply -f sidecar-deployment.yaml
```

Best for: per-pod log collection, application-level NGINX instances.

## DaemonSet Mode

Runs one nginx-clickhouse instance per node, reading from a `hostPath` log directory.

```sh
kubectl apply -f configmap.yaml
kubectl apply -f secret.yaml
kubectl apply -f daemonset.yaml
```

Best for: node-level NGINX (ingress controllers), centralized log collection.

## Configuration

**ConfigMap** (`configmap.yaml`): Contains the `config.yml` mounted into the container. Edit the ClickHouse connection, column mappings, and NGINX log format to match your setup.

**Secret** (`secret.yaml`): Stores ClickHouse credentials. In production, replace with your own secret management (ExternalSecrets, Vault, sealed-secrets).

## Kubernetes Metadata Enrichment

The manifests inject pod metadata via the [Downward API](https://kubernetes.io/docs/concepts/workloads/pods/downward-api/) as `ENRICHMENT_EXTRA_*` env vars:

| Env Var | Source | Extra Key |
|---|---|---|
| `ENRICHMENT_HOSTNAME` | `metadata.name` | (sets hostname) |
| `ENRICHMENT_EXTRA_POD_NAMESPACE` | `metadata.namespace` | `pod_namespace` |
| `ENRICHMENT_EXTRA_NODE_NAME` | `spec.nodeName` | `node_name` |
| `ENRICHMENT_EXTRA_POD_IP` | `status.podIP` | `pod_ip` |

Map these to ClickHouse columns via `_extra.<key>` in the column mapping:

```yaml
columns:
  PodNamespace: _extra.pod_namespace
  NodeName: _extra.node_name
  PodIP: _extra.pod_ip
```

## Prometheus Integration

All manifests include pod annotations for annotation-based scraping:

```yaml
prometheus.io/scrape: "true"
prometheus.io/port: "2112"
prometheus.io/path: "/metrics"
```

For Prometheus Operator, apply the ServiceMonitor:

```sh
kubectl apply -f servicemonitor.yaml
```

## Customization

- **Resources**: Uncomment and adjust `resources` blocks in the Deployment/DaemonSet.
- **Node selection**: Uncomment `nodeSelector` and `tolerations` in the DaemonSet.
- **TLS**: Add TLS env vars to the container spec (see main [README](../../README.md#tls--secure-connections)).
- **Disk buffer**: Add a `PersistentVolumeClaim` volume and set `BUFFER_TYPE=disk` + `BUFFER_DISK_PATH`.
