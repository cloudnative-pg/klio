## Packages
- [klio.cnpg.io/v1alpha1](#kliocnpgiov1alpha1)


## klio.cnpg.io/v1alpha1

Package v1alpha1 contains API Schema definitions for the klio v1alpha1 API group.

### Resource Types
- [PluginConfiguration](#pluginconfiguration)
- [RecoverySource](#recoverysource)
- [RecoverySourceList](#recoverysourcelist)
- [Server](#server)



#### CacheConfiguration



CacheConfiguration defines the configuration for the cache directory.



_Appears in:_
- [RecoverySourceStorageConfiguration](#recoverysourcestorageconfiguration)
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


#### ImageConfiguration



ImageConfiguration contains the information needed to download
the Klio image.



_Appears in:_
- [RecoverySourceSpec](#recoverysourcespec)
- [ServerSpec](#serverspec)

| Field | Description | Required | Default | Validation |
| --- | --- | --- | --- | --- |
| `image` _string_ | Image is the image to be used for the Klio server | True |  |  |
| `imagePullPolicy` _[PullPolicy](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.35/#pullpolicy-v1-core)_ | ImagePullPolicy defines the policy for pulling the image |  | IfNotPresent | Optional: \{\} <br /> |
| `imagePullSecrets` _[LocalObjectReference](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.35/#localobjectreference-v1-core) array_ | ImagePullSecrets is an optional list of references to secrets in the same namespace to use for pulling any of the<br />images |  |  | Optional: \{\} <br /> |


#### PluginConfiguration



PluginConfiguration is the Schema for the client configuration API.





| Field | Description | Required | Default | Validation |
| --- | --- | --- | --- | --- |
| `apiVersion` _string_ | `klio.cnpg.io/v1alpha1` | True | | |
| `kind` _string_ | `PluginConfiguration` | True | | |
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.35/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. | True |  |  |
| `spec` _[PluginConfigurationSpec](#pluginconfigurationspec)_ |  | True |  |  |
| `status` _[PluginConfigurationStatus](#pluginconfigurationstatus)_ |  |  |  | Optional: \{\} <br /> |


#### PluginConfigurationSpec



PluginConfigurationSpec defines the desired state of client configuration.



_Appears in:_
- [PluginConfiguration](#pluginconfiguration)

| Field | Description | Required | Default | Validation |
| --- | --- | --- | --- | --- |
| `serverAddress` _string_ | ServerAddress is the address of the Klio server | True |  | MinLength: 1 <br />Required: \{\} <br /> |
| `tier1` _[Tier1PluginConfiguration](#tier1pluginconfiguration)_ | Tier1 is the Tier 1 configuration |  |  | Optional: \{\} <br /> |
| `tier2` _[Tier2PluginConfiguration](#tier2pluginconfiguration)_ | Tier2 is the Tier 2 configuration |  |  | Optional: \{\} <br /> |
| `clientSecretName` _string_ | ClientSecretName is the name of the secret containing the client credentials | True |  | MinLength: 1 <br />Required: \{\} <br /> |
| `serverSecretName` _string_ | ServerSecretName is the name of the secret containing the server TLS certificate | True |  | MinLength: 1 <br />Required: \{\} <br /> |
| `clusterName` _string_ | ClusterName is the name of the PostgreSQL cluster we are connecting to |  |  | Optional: \{\} <br /> |
| `pprof` _boolean_ | Pprof enables the pprof endpoint for performance profiling |  |  | Optional: \{\} <br /> |
| `containers` _[Container](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.35/#container-v1-core) array_ | Containers allows defining a list of containers that will be merged with the Klio sidecar containers.<br />This enables users to customize the sidecars with additional environment variables, volume mounts,<br />resource limits, and other container settings without polluting the PostgreSQL container environment.<br />Merge behavior:<br />- Containers are matched by name (klio-plugin, klio-wal, klio-restore)<br />- User customizations serve as the base<br />- Klio required values (name, args, CONTAINER_NAME env var) always override user values<br />- User-defined environment variables and volume mounts are preserved<br />- Template defaults are applied only for fields not set by the user or Klio |  |  | MaxItems: 3 <br />Optional: \{\} <br /> |


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
| `pvcTemplate` _[PersistentVolumeClaimSpec](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.35/#persistentvolumeclaimspec-v1-core)_ | PersistentVolumeClaimTemplate is used to generate the configuration for<br />the PVC hosting the work queue. | True |  |  |


#### RecoverySource



RecoverySource is the Schema for the recovery source API.



_Appears in:_
- [RecoverySourceList](#recoverysourcelist)

| Field | Description | Required | Default | Validation |
| --- | --- | --- | --- | --- |
| `apiVersion` _string_ | `klio.cnpg.io/v1alpha1` | True | | |
| `kind` _string_ | `RecoverySource` | True | | |
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.35/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. | True |  |  |
| `spec` _[RecoverySourceSpec](#recoverysourcespec)_ |  | True |  |  |
| `status` _[RecoverySourceStatus](#recoverysourcestatus)_ |  |  |  | Optional: \{\} <br /> |


#### RecoverySourceList



RecoverySourceList contains a list of RecoverySources.





| Field | Description | Required | Default | Validation |
| --- | --- | --- | --- | --- |
| `apiVersion` _string_ | `klio.cnpg.io/v1alpha1` | True | | |
| `kind` _string_ | `RecoverySourceList` | True | | |
| `metadata` _[ListMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.35/#listmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. | True |  |  |
| `items` _[RecoverySource](#recoverysource) array_ |  | True |  |  |


#### RecoverySourceSpec



RecoverySourceSpec defines a remote Klio tier2 to be used as a
recovery source.



_Appears in:_
- [RecoverySource](#recoverysource)

| Field | Description | Required | Default | Validation |
| --- | --- | --- | --- | --- |
| `image` _string_ | Image is the image to be used for the Klio server | True |  |  |
| `imagePullPolicy` _[PullPolicy](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.35/#pullpolicy-v1-core)_ | ImagePullPolicy defines the policy for pulling the image |  | IfNotPresent | Optional: \{\} <br /> |
| `imagePullSecrets` _[LocalObjectReference](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.35/#localobjectreference-v1-core) array_ | ImagePullSecrets is an optional list of references to secrets in the same namespace to use for pulling any of the<br />images |  |  | Optional: \{\} <br /> |
| `tlsSecretName` _string_ | TLSSecretName is the name of the Kubernetes secret containing the server-side certificate<br />to be used for the Klio server. | True |  |  |
| `caSecretName` _string_ | ClientCASecretName is the name of the Kubernetes secret containing the CA certificate<br />to be used by the Klio server to validate the users. | True |  |  |
| `tier2` _[Tier2Configuration](#tier2configuration)_ | Tier2 is the tier 2 configuration to be used by this recovery source. | True |  |  |
| `storage` _[RecoverySourceStorageConfiguration](#recoverysourcestorageconfiguration)_ | Storage is the storage resources to be used<br />for this Klio recovery source. | True |  |  |
| `template` _[PodTemplateSpec](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.35/#podtemplatespec-v1-core)_ | Template to override the default StatefulSet of the Klio recovery source.<br />WARNING: Modifying this template may break the server functionality if not done carefully.<br />This field is primarily intended for advanced configuration such as telemetry setup.<br />Use at your own risk and ensure thorough testing before applying changes. |  |  | Optional: \{\} <br /> |


#### RecoverySourceStatus



RecoverySourceStatus defines the observed state of recovery source.



_Appears in:_
- [RecoverySource](#recoverysource)



#### RecoverySourceStorageConfiguration



RecoverySourceStorageConfiguration defines the storage
to be used for this recovery source.



_Appears in:_
- [RecoverySourceSpec](#recoverysourcespec)

| Field | Description | Required | Default | Validation |
| --- | --- | --- | --- | --- |
| `cache` _[CacheConfiguration](#cacheconfiguration)_ | Cache is the configuration of the PVC that should be<br />used for the cache. | True |  |  |


#### RetentionPolicy



RetentionPolicy defines how many backups we should keep.



_Appears in:_
- [Tier1PluginConfiguration](#tier1pluginconfiguration)
- [Tier2PluginConfiguration](#tier2pluginconfiguration)

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
| `prefix` _string_ | Prefix is the prefix to be used for the stored files |  |  | Optional: \{\} <br /> |
| `endpoint` _string_ | Endpoint is the endpoint to be used |  |  | Optional: \{\} <br /> |
| `region` _string_ | Region is the region to be used |  |  | Optional: \{\} <br /> |
| `accessKeyId` _[SecretKeySelector](#secretkeyselector)_ | The S3 access key ID |  |  | Optional: \{\} <br /> |
| `secretAccessKey` _[SecretKeySelector](#secretkeyselector)_ | The S3 access key |  |  | Optional: \{\} <br /> |
| `sessionToken` _[SecretKeySelector](#secretkeyselector)_ | The S3 session token |  |  | Optional: \{\} <br /> |
| `customCaBundle` _[SecretKeySelector](#secretkeyselector)_ | A pointer to a custom CA bundle |  |  | Optional: \{\} <br /> |


#### Server



Server is the Schema for the servers API.





| Field | Description | Required | Default | Validation |
| --- | --- | --- | --- | --- |
| `apiVersion` _string_ | `klio.cnpg.io/v1alpha1` | True | | |
| `kind` _string_ | `Server` | True | | |
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.35/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. | True |  |  |
| `spec` _[ServerSpec](#serverspec)_ |  | True |  |  |
| `status` _[ServerStatus](#serverstatus)_ |  |  |  | Optional: \{\} <br /> |


#### ServerSpec



ServerSpec defines the desired state of Server.



_Appears in:_
- [Server](#server)

| Field | Description | Required | Default | Validation |
| --- | --- | --- | --- | --- |
| `image` _string_ | Image is the image to be used for the Klio server | True |  |  |
| `imagePullPolicy` _[PullPolicy](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.35/#pullpolicy-v1-core)_ | ImagePullPolicy defines the policy for pulling the image |  | IfNotPresent | Optional: \{\} <br /> |
| `imagePullSecrets` _[LocalObjectReference](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.35/#localobjectreference-v1-core) array_ | ImagePullSecrets is an optional list of references to secrets in the same namespace to use for pulling any of the<br />images |  |  | Optional: \{\} <br /> |
| `tlsSecretName` _string_ | TLSSecretName is the name of the Kubernetes secret containing the server-side certificate<br />to be used for the Klio server. | True |  |  |
| `caSecretName` _string_ | ClientCASecretName is the name of the Kubernetes secret containing the CA certificate<br />to be used by the Klio server to validate the users. | True |  |  |
| `cacheConfiguration` _[CacheConfiguration](#cacheconfiguration)_ | CacheConfiguration is the configuration of the PVC that should be<br />used for the cache | True |  |  |
| `dataConfiguration` _[DataConfiguration](#dataconfiguration)_ | DataConfiguration is the configuration of the PVC that should be used<br />for the base backups | True |  |  |
| `queueConfiguration` _[QueueConfiguration](#queueconfiguration)_ | QueueConfiguration is the configuration of the PVC that should host<br />the task queue. |  |  | Optional: \{\} <br /> |
| `password` _[SecretKeySelector](#secretkeyselector)_ | Password is a reference to a secret containing the Klio password | True |  |  |
| `tier2` _[Tier2Configuration](#tier2configuration)_ | Tier2 is the Tier 2 configuration | True |  |  |
| `template` _[PodTemplateSpec](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.35/#podtemplatespec-v1-core)_ | Template to override the default StatefulSet of the Klio server.<br />WARNING: Modifying this template may break the server functionality if not done carefully.<br />This field is primarily intended for advanced configuration such as telemetry setup.<br />Use at your own risk and ensure thorough testing before applying changes. |  |  | Optional: \{\} <br /> |


#### ServerStatus



ServerStatus defines the observed state of Server.



_Appears in:_
- [Server](#server)



#### TLSConfiguration



TLSConfiguration contains the information needed to configure
the PKI infrastructure of the Klio server.



_Appears in:_
- [RecoverySourceSpec](#recoverysourcespec)
- [ServerSpec](#serverspec)

| Field | Description | Required | Default | Validation |
| --- | --- | --- | --- | --- |
| `tlsSecretName` _string_ | TLSSecretName is the name of the Kubernetes secret containing the server-side certificate<br />to be used for the Klio server. | True |  |  |
| `caSecretName` _string_ | ClientCASecretName is the name of the Kubernetes secret containing the CA certificate<br />to be used by the Klio server to validate the users. | True |  |  |


#### Tier1PluginConfiguration



Tier1PluginConfiguration configures tier1 backup and recovery settings.



_Appears in:_
- [PluginConfigurationSpec](#pluginconfigurationspec)

| Field | Description | Required | Default | Validation |
| --- | --- | --- | --- | --- |
| `retention` _[RetentionPolicy](#retentionpolicy)_ | RetentionPolicy defines how many backups we should keep |  |  | Optional: \{\} <br /> |


#### Tier2Configuration



Tier2Configuration is the tier 2 configuration.



_Appears in:_
- [RecoverySourceSpec](#recoverysourcespec)
- [ServerSpec](#serverspec)

| Field | Description | Required | Default | Validation |
| --- | --- | --- | --- | --- |
| `s3` _[S3Configuration](#s3configuration)_ | S3 contains the configuration parameters for an S3-based tier 2 | True |  |  |
| `encryptionPassword` _[SecretKeySelector](#secretkeyselector)_ | EncryptionPassword is a pointer to the key in a secret containing the encryption password. | True |  |  |


#### Tier2PluginConfiguration



Tier2PluginConfiguration configures tier2 backup and recovery settings.



_Appears in:_
- [PluginConfigurationSpec](#pluginconfigurationspec)

| Field | Description | Required | Default | Validation |
| --- | --- | --- | --- | --- |
| `enableBackup` _boolean_ | EnableBackup controls whether WAL and base backups should be stored in tier2 |  |  | Optional: \{\} <br /> |
| `enableRecovery` _boolean_ | EnableRecovery controls whether tier2 should be included in the recovery source list |  |  | Optional: \{\} <br /> |
| `retention` _[RetentionPolicy](#retentionpolicy)_ | RetentionPolicy defines how many backups we should keep |  |  | Optional: \{\} <br /> |


