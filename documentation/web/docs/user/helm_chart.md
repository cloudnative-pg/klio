---
title: EDB Klio Operator Helm Chart
sidebar_position: 90
---
![Version: 0.0.5](https://img.shields.io/badge/Version-0.0.5-informational?style=flat-square) ![Type: application](https://img.shields.io/badge/Type-application-informational?style=flat-square) ![AppVersion: 0.0.5](https://img.shields.io/badge/AppVersion-0.0.5-informational?style=flat-square)

The EDB Klio Operator Helm chart from EDB allows you to deploy the Klio
Operator in your Kubernetes cluster. It is distributed as an OCI image. You can
install it with the following command:

```sh
helm install klio-operator oci://ghcr.io/enterprisedb/klio-operator-chart --version $VERSION
```

The chart is designed to be customizable, allowing you to configure various aspects of the Klio Operator deployment,
passing in values through a custom `values.yaml` file or using the `--set` flag during installation.
See the [Helm documentation](https://helm.sh/docs/) for more details on how to customize your installation.

## Values

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| certmanager.createMetricsCertificate | bool | `true` | Create certificates for the metrics service. |
| certmanager.createPluginClientCertificate | bool | `true` | Create certificates for the plugin client. |
| certmanager.createPluginServerCertificate | bool | `true` | Create certificates for the plugin server. |
| certmanager.duration | string | `"2160h"` | The duration of the certificates. |
| certmanager.enable | bool | `true` | Enable cert-manager integration for certificate creation. |
| certmanager.renewBefore | string | `"360h"` | The renew before time for the certificates. |
| controllerManager.affinity | object | `{}` | Affinity rules for the operator deployment. |
| controllerManager.manager.args | list | `["--metrics-bind-address=:8443","--leader-elect","--health-probe-bind-address=:8081","--plugin-server-cert=/pluginServer/tls.crt","--plugin-server-key=/pluginServer/tls.key","--plugin-client-cert=/pluginClient/tls.crt","--plugin-server-address=:9090","--custom-cnpg-group=postgresql.cnpg.io"]` | List of command line arguments to pass to the controller manager. |
| controllerManager.manager.containerSecurityContext | object | `{"allowPrivilegeEscalation":false,"capabilities":{"drop":["ALL"]}}` | The security context for the controller manager container. |
| controllerManager.manager.env | object | `{"SIDECAR_IMAGE":"registry.dev:5000/klio-testing:dev"}` | The environment variables to set in the controller manager container. |
| controllerManager.manager.image.pullPolicy | string | `"Always"` | The controller manager container imagePullPolicy. |
| controllerManager.manager.image.pullSecrets | list | `[]` | The list of imagePullSecrets. |
| controllerManager.manager.image.repository | string | `"controller"` | The image to use for the controller manager container. |
| controllerManager.manager.image.tag | string | `"latest"` | The tag to use for the controller manager container image. |
| controllerManager.manager.livenessProbe | object | `{"httpGet":{"path":"/healthz","port":8081},"initialDelaySeconds":15,"periodSeconds":20}` | Liveness probe configuration. |
| controllerManager.manager.readinessProbe | object | `{"httpGet":{"path":"/readyz","port":8081},"initialDelaySeconds":5,"periodSeconds":10}` | Readiness probe configuration. |
| controllerManager.manager.resources | object | `{"limits":{"cpu":"500m","memory":"128Mi"},"requests":{"cpu":"10m","memory":"64Mi"}}` | The resources to allocate. |
| controllerManager.nodeSelector | object | `{}` | NodeSelector for the operator deployment. |
| controllerManager.podSecurityContext | object | `{"runAsNonRoot":true,"seccompProfile":{"type":"RuntimeDefault"}}` | The security context for the controller manager pod. |
| controllerManager.priorityClassName | string | `""` | Priority class name for the controller manager pod. |
| controllerManager.serviceAccount.annotations | object | `{}` | The annotations to add to the service account. |
| controllerManager.tolerations | list | `[]` | Tolerations for the operator deployment. |
| controllerManager.topologySpreadConstraints | list | `[]` | Topology Spread Constraints for the operator deployment. |
| fullnameOverride | string | `""` | Override the fully qualified name of the Helm Chart. |
| kubernetesClusterDomain | string | `"cluster.local"` | The domain for the Kubernetes cluster. |
| metricsService.enable | bool | `true` | Enable the metrics service for the controller manager. |
| metricsService.metricsServiceSecret | string | `"klio-metrics-server-cert"` | The name of the secret containing the TLS certificate for the metrics service. |
| metricsService.ports | list | `[{"name":"https","port":8443,"protocol":"TCP","targetPort":8443}]` | The port the metrics service will listen on. |
| metricsService.type | string | `"ClusterIP"` | Service type for the metrics service. |
| nameOverride | string | `"klio"` | Override the name of the Helm Chart. |
| plugin.clientSecret | string | `"klio-plugin-client-tls"` | The Client TLS certificate. |
| plugin.name | string | `"klio.cnpg.io"` | The name the plugin will use to register itself with the CNPG Operator. |
| plugin.port | int | `9090` | The port the plugin will listen on. It must match the "--plugin-server-address" argument. |
| plugin.serverSecret | string | `"klio-plugin-server-tls"` | The Server TLS certificate. |
| prometheus.enable | bool | `true` | To enable a ServiceMonitor to export metrics to Prometheus set true. |

----------------------------------------------
Autogenerated from chart metadata using [helm-docs v1.14.2](https://github.com/norwoodj/helm-docs/releases/v1.14.2)
