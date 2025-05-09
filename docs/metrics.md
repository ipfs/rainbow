## rainbow metrics

By default, a prometheus endpoint is exposed by Rainbow at `http://127.0.0.1:8091/debug/metrics/prometheus`.

It includes default [Prometheus Glient metrics](https://prometheus.io/docs/guides/go-application/) + rainbow-specific listed below.

### Delegated HTTP Routing (`/routing/v1`) client

Metrics from `boxo/routing/http/client` are exposed with `ipfs_` prefix:

- Histogram: the latency of operations by the routing HTTP client:
  - `ipfs_routing_http_client_latency_bucket{code,error,host,operation,le}`
  - `ipfs_routing_http_client_latency_sum{code,error,host,operation}`
  - `ipfs_routing_http_client_latency_count{code,error,host,operation}`
- Histogram: the number of elements in a response collection
  - `ipfs_routing_http_client_length_bucket{host,operation,le}`
  - `ipfs_routing_http_client_length_sum{host,operation}`
  - `ipfs_routing_http_client_length_count{host,operation}`

