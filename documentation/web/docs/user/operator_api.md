# API Reference

## Packages
- [klio.cnpg.io/v1alpha1](#kliocnpgiov1alpha1)


## klio.cnpg.io/v1alpha1

Package v1alpha1 contains API Schema definitions for the klio v1alpha1 API group.

### Resource Types
- [PluginConfiguration](#pluginconfiguration)
- [Server](#server)



#### BaseConfiguration



BaseConfiguration defines the configuration for the base server.



_Appears in:_
- [ServerSpec](#serverspec)

| Field | Description | Required | Default | Validation |
| --- | --- | --- | --- | --- |
| `resources` _[ResourceRequirements](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.32/#resourcerequirements-v1-core)_ | Resources defines the resource requirements for the Kopia server |  |  |  |
| `adminUser` _[LocalObjectReference](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.32/#localobjectreference-v1-core)_ | AdminUser is a reference to a secret of type 'kubernetes.io/basic-auth' |  |  |  |


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
| `pvcTemplate` _[PersistentVolumeClaimSpec](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.32/#persistentvolumeclaimspec-v1-core)_ | Template to be used to generate the Persistent Volume Claim needed for the data folder,<br />containing base backups and WAL files. | True |  |  |


#### PluginConfiguration



PluginConfiguration is the Schema for the client configuration API.





| Field | Description | Required | Default | Validation |
| --- | --- | --- | --- | --- |
| `apiVersion` _string_ | `klio.cnpg.io/v1alpha1` | True | | |
| `kind` _string_ | `PluginConfiguration` | True | | |
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.32/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. | True |  |  |
| `spec` _[PluginConfigurationSpec](#pluginconfigurationspec)_ |  | True |  | AtMostOneOf: [backupRef backupId] <br /> |
| `status` _[PluginConfigurationStatus](#pluginconfigurationstatus)_ |  |  |  |  |


#### PluginConfigurationSpec



PluginConfigurationSpec defines the desired state of client configuration.

_Validation:_
- AtMostOneOf: [backupRef backupId]

_Appears in:_
- [PluginConfiguration](#pluginconfiguration)

| Field | Description | Required | Default | Validation |
| --- | --- | --- | --- | --- |
| `serverAddress` _string_ | ServerAddress is the address of the Klio server in the format host:port or host | True |  | MinLength: 1 <br />Required: \{\} <br /> |
| `clientSecretName` _string_ | ClientSecretName is the name of the secret containing the client credentials | True |  | MinLength: 1 <br />Required: \{\} <br /> |
| `serverSecretName` _string_ | ServerSecretName is the name of the secret containing the server TLS certificate | True |  | MinLength: 1 <br />Required: \{\} <br /> |
| `clusterName` _string_ | ClusterName is the name of the PostgreSQL cluster we are connecting to |  |  |  |
| `backupRef` _string_ | BackupRef is the reference to the backup we should use for restores |  |  |  |
| `backupId` _string_ | BackupID is the ID of the backup we should use for restores |  |  |  |
| `pprof` _boolean_ | Pprof enables the pprof endpoint for performance profiling |  |  |  |
| `metricsAddressInstance` _string_ | MetricsAddressInstance is the address where the metrics for the instance should be exposed |  |  |  |
| `metricsAddressRestore` _string_ | MetricsAddressRestore is the address where the metrics for the restore should be exposed |  |  |  |
| `metricsAddressSendWal` _string_ | MetricsAddressSendWal is the address where the metrics for the WAL sender should be exposed |  |  |  |
| `retention` _[RetentionPolicy](#retentionpolicy)_ | RetentionPolicy defines how many backups we should keep |  |  |  |


#### PluginConfigurationStatus



PluginConfigurationStatus defines the observed state of ClientConfig.



_Appears in:_
- [PluginConfiguration](#pluginconfiguration)



#### RetentionPolicy



RetentionPolicy defines how many backups we should keep.



_Appears in:_
- [PluginConfigurationSpec](#pluginconfigurationspec)

| Field | Description | Required | Default | Validation |
| --- | --- | --- | --- | --- |
| `keepLatest` _integer_ | KeepLatest is the number of latest backups to keep<br />optional | True |  |  |
| `keepAnnual` _integer_ | KeepAnnual is the number of annual backups to keep<br />optional | True |  |  |
| `keepMonthly` _integer_ | KeepMonthly is the number of monthly backups to keep<br />optional | True |  |  |
| `keepWeekly` _integer_ | KeepWeekly is the number of weekly backups to keep<br />optional | True |  |  |
| `keepDaily` _integer_ | KeepDaily is the number of daily backups to keep<br />optional | True |  |  |
| `keepHourly` _integer_ | KeepHourly is the number of hourly backups to keep<br />optional | True |  |  |


#### Server



Server is the Schema for the servers API.





| Field | Description | Required | Default | Validation |
| --- | --- | --- | --- | --- |
| `apiVersion` _string_ | `klio.cnpg.io/v1alpha1` | True | | |
| `kind` _string_ | `Server` | True | | |
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.32/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. | True |  |  |
| `spec` _[ServerSpec](#serverspec)_ |  | True |  |  |
| `status` _[ServerStatus](#serverstatus)_ |  |  |  |  |


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
| `caSecretName` _string_ | ClientCASecretName is the name of the Kubernetes secret containing the CA certificate<br />to be used by the Klio server to validate the users. | True |  |  |
| `resources` _[ResourceRequirements](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.32/#resourcerequirements-v1-core)_ | Resources defines the resource requirements for the Klio server |  |  |  |
| `cacheConfiguration` _[CacheConfiguration](#cacheconfiguration)_ | CacheConfiguration is the configuration of the PVC that should be<br />used for the cache | True |  |  |
| `dataConfiguration` _[DataConfiguration](#dataconfiguration)_ | DataConfiguration is the configuration of the PVC that should be used<br />for the base backups | True |  |  |
| `password` _[SecretKeySelector](#secretkeyselector)_ | Password is a reference to a secret containing the Klio password | True |  |  |
| `template` _[PodTemplateSpec](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.32/#podtemplatespec-v1-core)_ | Template to override the default StatefulSet of the Klio server.<br />WARNING: Modifying this template may break the server functionality if not done carefully.<br />This field is primarily intended for advanced configuration such as telemetry setup.<br />Use at your own risk and ensure thorough testing before applying changes. |  |  |  |


#### ServerStatus



ServerStatus defines the observed state of Server.



_Appears in:_
- [Server](#server)



