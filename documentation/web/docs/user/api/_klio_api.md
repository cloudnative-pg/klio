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
| `resources` _[ResourceRequirements](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.35/#resourcerequirements-v1-core)_ | Resources defines the resource requirements for the Kopia server |  |  |  |


#### CacheConfiguration



CacheConfiguration defines the configuration for the cache directory.



_Appears in:_
- [ServerSpec](#serverspec)

| Field | Description | Required | Default | Validation |
| --- | --- | --- | --- | --- |
| `pvcTemplate` _[PersistentVolumeClaimSpec](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.35/#persistentvolumeclaimspec-v1-core)_ |  | True |  |  |


#### DataConfiguration



DataConfiguration defines the configuration for the data directory.



_Appears in:_
- [ServerSpec](#serverspec)

| Field | Description | Required | Default | Validation |
| --- | --- | --- | --- | --- |
| `pvcTemplate` _[PersistentVolumeClaimSpec](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.35/#persistentvolumeclaimspec-v1-core)_ | Template to be used to generate the Persistent Volume Claim needed for the data folder,<br />containing base backups and WAL files. | True |  |  |


#### PluginConfiguration



PluginConfiguration is the Schema for the client configuration API.





| Field | Description | Required | Default | Validation |
| --- | --- | --- | --- | --- |
| `apiVersion` _string_ | `klio.cnpg.io/v1alpha1` | True | | |
| `kind` _string_ | `PluginConfiguration` | True | | |
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.35/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. | True |  |  |
| `spec` _[PluginConfigurationSpec](#pluginconfigurationspec)_ |  | True |  |  |
| `status` _[PluginConfigurationStatus](#pluginconfigurationstatus)_ |  |  |  |  |


#### PluginConfigurationSpec



PluginConfigurationSpec defines the desired state of client configuration.



_Appears in:_
- [PluginConfiguration](#pluginconfiguration)

| Field | Description | Required | Default | Validation |
| --- | --- | --- | --- | --- |
| `serverAddress` _string_ | ServerAddress is the address of the Klio server | True |  | MinLength: 1 <br />Required: \{\} <br /> |
| `tier2` _boolean_ | Tier2 enables backup lookup in tier 2. | True |  |  |
| `clientSecretName` _string_ | ClientSecretName is the name of the secret containing the client credentials | True |  | MinLength: 1 <br />Required: \{\} <br /> |
| `serverSecretName` _string_ | ServerSecretName is the name of the secret containing the server TLS certificate | True |  | MinLength: 1 <br />Required: \{\} <br /> |
| `clusterName` _string_ | ClusterName is the name of the PostgreSQL cluster we are connecting to |  |  |  |
| `pprof` _boolean_ | Pprof enables the pprof endpoint for performance profiling |  |  |  |
| `retention` _[RetentionPolicy](#retentionpolicy)_ | RetentionPolicy defines how many backups we should keep |  |  |  |
| `containers` _[Container](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.35/#container-v1-core) array_ | Containers allows defining a list of containers that will be merged with the Klio sidecar containers.<br />This enables users to customize the sidecars with additional environment variables, volume mounts,<br />resource limits, and other container settings without polluting the PostgreSQL container environment.<br />Merge behavior:<br />- Containers are matched by name (klio-plugin, klio-wal, klio-restore)<br />- User customizations serve as the base<br />- Klio required values (name, args, CONTAINER_NAME env var) always override user values<br />- User-defined environment variables and volume mounts are preserved<br />- Template defaults are applied only for fields not set by the user or Klio |  |  | MaxItems: 3 <br /> |


#### PluginConfigurationStatus



PluginConfigurationStatus defines the observed state of ClientConfig.



_Appears in:_
- [PluginConfiguration](#pluginconfiguration)



#### QueueConfiguration



QueueConfiguration defines the configuration for the directory hosting the
task queue.



_Appears in:_
- [ServerSpec](#serverspec)

| Field | Description | Required | Default | Validation |
| --- | --- | --- | --- | --- |
| `resources` _[ResourceRequirements](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.35/#resourcerequirements-v1-core)_ | QueueResources defines the resource requirements for the NATS server |  |  |  |
| `pvcTemplate` _[PersistentVolumeClaimSpec](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.35/#persistentvolumeclaimspec-v1-core)_ | PersistentVolumeClaimTemplate is used to generate the configuration for<br />the PVC hosting the work queue. | True |  |  |


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


#### S3Configuration



S3Configuration is the configuration to a S3 defined tier 2.



_Appears in:_
- [Tier2Configuration](#tier2configuration)

| Field | Description | Required | Default | Validation |
| --- | --- | --- | --- | --- |
| `bucketName` _string_ | BucketName is the name of the bucket | True |  |  |
| `prefix` _string_ | Prefix is the prefix to be used for the stored files |  |  |  |
| `endpoint` _string_ | Endpoint is the endpoint to be used |  |  |  |
| `region` _string_ | Region is the region to be used |  |  |  |
| `walEncryptionPassword` _[SecretKeySelector](#secretkeyselector)_ | WALEncryptionPassword is a pointer to the key in a secret containing the encryption password. | True |  |  |
| `accessKeyId` _[SecretKeySelector](#secretkeyselector)_ | The S3 access key ID |  |  |  |
| `secretAccessKey` _[SecretKeySelector](#secretkeyselector)_ | The S3 access key |  |  |  |
| `sessionToken` _[SecretKeySelector](#secretkeyselector)_ | The S3 session token |  |  |  |
| `customCaBundle` _[SecretKeySelector](#secretkeyselector)_ | A pointer to a custom CA bundle |  |  |  |


#### Server



Server is the Schema for the servers API.





| Field | Description | Required | Default | Validation |
| --- | --- | --- | --- | --- |
| `apiVersion` _string_ | `klio.cnpg.io/v1alpha1` | True | | |
| `kind` _string_ | `Server` | True | | |
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.35/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. | True |  |  |
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
| `imagePullPolicy` _[PullPolicy](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.35/#pullpolicy-v1-core)_ | ImagePullPolicy defines the policy for pulling the image |  | IfNotPresent |  |
| `imagePullSecrets` _[LocalObjectReference](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.35/#localobjectreference-v1-core) array_ | ImagePullSecrets is an optional list of references to secrets in the same namespace to use for pulling any of the<br />images |  |  |  |
| `tlsSecretName` _string_ | TLSSecretName is the name of the Kubernetes secret containing the server-side certificate<br />to be used for the Klio server. | True |  |  |
| `caSecretName` _string_ | ClientCASecretName is the name of the Kubernetes secret containing the CA certificate<br />to be used by the Klio server to validate the users. | True |  |  |
| `resources` _[ResourceRequirements](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.35/#resourcerequirements-v1-core)_ | Resources defines the resource requirements for the Klio server |  |  |  |
| `cacheConfiguration` _[CacheConfiguration](#cacheconfiguration)_ | CacheConfiguration is the configuration of the PVC that should be<br />used for the cache | True |  |  |
| `dataConfiguration` _[DataConfiguration](#dataconfiguration)_ | DataConfiguration is the configuration of the PVC that should be used<br />for the base backups | True |  |  |
| `queueConfiguration` _[QueueConfiguration](#queueconfiguration)_ | QueueConfiguration is the configuration of the PVC that should host<br />the task queue. |  |  |  |
| `password` _[SecretKeySelector](#secretkeyselector)_ | Password is a reference to a secret containing the Klio password | True |  |  |
| `tier2` _[Tier2Configuration](#tier2configuration)_ | Tier2 is the Tier 2 configuration | True |  |  |
| `template` _[PodTemplateSpec](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.35/#podtemplatespec-v1-core)_ | Template to override the default StatefulSet of the Klio server.<br />WARNING: Modifying this template may break the server functionality if not done carefully.<br />This field is primarily intended for advanced configuration such as telemetry setup.<br />Use at your own risk and ensure thorough testing before applying changes. |  |  |  |


#### ServerStatus



ServerStatus defines the observed state of Server.



_Appears in:_
- [Server](#server)



#### Tier2Configuration



Tier2Configuration is the tier 2 configuration.



_Appears in:_
- [ServerSpec](#serverspec)

| Field | Description | Required | Default | Validation |
| --- | --- | --- | --- | --- |
| `s3` _[S3Configuration](#s3configuration)_ | S3 contains the configuration parameters for an S3-based tier 2 | True |  |  |


