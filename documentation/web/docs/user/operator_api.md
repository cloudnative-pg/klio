# API Reference

## Packages
- [klio.cnpg.io/v1alpha1](#kliocnpgiov1alpha1)


## klio.cnpg.io/v1alpha1

Package v1alpha1 contains API Schema definitions for the klio v1alpha1 API group.

### Resource Types
- [Server](#server)



#### BaseConfiguration



BaseConfiguration defines the configuration for the Kopia server.



_Appears in:_
- [ServerSpec](#serverspec)

| Field | Description | Required | Default | Validation |
| --- | --- | --- | --- | --- |
| `resources` _[ResourceRequirements](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.32/#resourcerequirements-v1-core)_ | Resources defines the resource requirements for the Kopia server |  |  |  |
| `adminUser` _[LocalObjectReference](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.32/#localobjectreference-v1-core)_ | AdminUser is a reference to a secret of type 'kubernetes.io/basic-auth' |  |  |  |
| `envs` _[EnvConfiguration](#envconfiguration)_ | Envs defines the environment variables to be set in the containers |  |  |  |


#### CacheConfiguration



CacheConfiguration defines the configuration for the cache directory.



_Appears in:_
- [ServerSpec](#serverspec)

| Field | Description | Required | Default | Validation |
| --- | --- | --- | --- | --- |
| `pvcTemplate` _[PersistentVolumeClaimSpec](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.32/#persistentvolumeclaimspec-v1-core)_ |  | True |  |  |


#### DataConfiguration



DataConfiguration defines the configuration for the data directory.



_Appears in:_
- [ServerSpec](#serverspec)

| Field | Description | Required | Default | Validation |
| --- | --- | --- | --- | --- |
| `pvcTemplate` _[PersistentVolumeClaimSpec](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.32/#persistentvolumeclaimspec-v1-core)_ | Template to be used to generate the Persistent Volume Claim needed for data folder | True |  |  |


#### EnvConfiguration



EnvConfiguration defines the environment variables configuration for the Kopia containers.



_Appears in:_
- [BaseConfiguration](#baseconfiguration)

| Field | Description | Required | Default | Validation |
| --- | --- | --- | --- | --- |
| `common` _[EnvVar](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.32/#envvar-v1-core) array_ | Common contains environment variables common to all containers |  |  |  |
| `base` _[EnvVar](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.32/#envvar-v1-core) array_ | Base contains environment variables specific to the base container |  |  |  |
| `wal` _[EnvVar](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.32/#envvar-v1-core) array_ | WAL contains environment variables specific to the WAL container |  |  |  |


#### Observability



Observability defines the observability configuration for the Kopia server.



_Appears in:_
- [ServerSpec](#serverspec)

| Field | Description | Required | Default | Validation |
| --- | --- | --- | --- | --- |
| `openTelemetry` _[OpenTelemetryConfiguration](#opentelemetryconfiguration)_ | OpenTelemetry contains configuration for OpenTelemetry autoexport |  |  |  |


#### OpenTelemetryConfiguration



OpenTelemetryConfiguration defines the configuration for OpenTelemetry.



_Appears in:_
- [Observability](#observability)

| Field | Description | Required | Default | Validation |
| --- | --- | --- | --- | --- |
| `pvcProjectedSource` _[ProjectedVolumeSource](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.32/#projectedvolumesource-v1-core)_ | Mounts the projected volume at '/otel' directory. Common for all containers. |  |  |  |


#### Server



Server is the Schema for the servers API.





| Field | Description | Required | Default | Validation |
| --- | --- | --- | --- | --- |
| `apiVersion` _string_ | `klio.cnpg.io/v1alpha1` | True | | |
| `kind` _string_ | `Server` | True | | |
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.32/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. | True |  |  |
| `spec` _[ServerSpec](#serverspec)_ |  | True |  |  |
| `status` _[ServerStatus](#serverstatus)_ |  | True |  |  |


#### ServerSpec



ServerSpec defines the desired state of Server.



_Appears in:_
- [Server](#server)

| Field | Description | Required | Default | Validation |
| --- | --- | --- | --- | --- |
| `baseConfiguration` _[BaseConfiguration](#baseconfiguration)_ | BaseConfiguration is the configuration of the Kopia server |  |  |  |
| `image` _string_ | Image is the image to be used for the Klio server | True |  |  |
| `imagePullPolicy` _[PullPolicy](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.32/#pullpolicy-v1-core)_ | ImagePullPolicy defines the policy for pulling the image |  | IfNotPresent |  |
| `imagePullSecrets` _[LocalObjectReference](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.32/#localobjectreference-v1-core) array_ | ImagePullSecrets is an optional list of references to secrets in the same namespace to use for pulling any of the<br />images |  |  |  |
| `tlsSecretName` _string_ | TLSSecretName is the name of the Kubernetes secret containing the server-side certificate<br />to be used for the Klio server. | True |  |  |
| `resources` _[ResourceRequirements](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.32/#resourcerequirements-v1-core)_ | Resources defines the resource requirements for the Klio server |  |  |  |
| `cacheConfiguration` _[CacheConfiguration](#cacheconfiguration)_ | CacheConfiguration is the configuration of the PVC that should be<br />used for the cache | True |  |  |
| `dataConfiguration` _[DataConfiguration](#dataconfiguration)_ | DataConfiguration is the configuration of the PVC that should be used<br />for the base backups | True |  |  |
| `password` _[SecretKeySelector](#secretkeyselector)_ | Password is a reference to a secret containing the Klio password | True |  |  |
| `users` _[LocalObjectReference](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.32/#localobjectreference-v1-core)_ | Users is a reference to a secret containing a htpasswd file at the 'htpasswd' key. | True |  |  |
| `observability` _[Observability](#observability)_ | Observability defines the observability configuration. | True |  |  |


#### ServerStatus



ServerStatus defines the observed state of Server.



_Appears in:_
- [Server](#server)



