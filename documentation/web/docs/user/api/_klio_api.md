## Packages
- [klio.cnpg.io/v1alpha1](#kliocnpgiov1alpha1)


## klio.cnpg.io/v1alpha1

Package v1alpha1 contains API Schema definitions for the klio v1alpha1 API group.

### Resource Types
- [PluginConfiguration](#pluginconfiguration)
- [Server](#server)



#### Cache



Cache defines the configuration for the cache directory.



_Appears in:_
- [Tier1Configuration](#tier1configuration)
- [Tier2Configuration](#tier2configuration)

| Field | Description | Required | Default | Validation |
| --- | --- | --- | --- | --- |
| `pvcTemplate` _[PersistentVolumeClaimSpec](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.37/#persistentvolumeclaimspec-v1-core)_ |  | True |  |  |


#### CompressionAlgorithm

_Underlying type:_ _string_

CompressionAlgorithm is the name of a Kopia compression algorithm.
The special value "none" disables compression.

_Validation:_
- Enum: [none deflate-best-compression deflate-best-speed deflate-default gzip gzip-best-compression gzip-best-speed pgzip pgzip-best-compression pgzip-best-speed s2-better s2-default s2-parallel-4 s2-parallel-8 zstd zstd-better-compression zstd-fastest]

_Appears in:_
- [CompressionPolicy](#compressionpolicy)



#### CompressionPolicy



CompressionPolicy configures the Kopia compression policy applied to base
backup data.
A `minSize` above a non-zero `maxSize` would match no file at all: Kopia
accepts such a policy and then silently skips compression for every file, so
it is rejected at admission instead.



_Appears in:_
- [Tier1Configuration](#tier1configuration)
- [Tier1PluginConfiguration](#tier1pluginconfiguration)
- [Tier2Configuration](#tier2configuration)
- [Tier2PluginConfiguration](#tier2pluginconfiguration)

| Field | Description | Required | Default | Validation |
| --- | --- | --- | --- | --- |
| `algorithm` _[CompressionAlgorithm](#compressionalgorithm)_ | Algorithm is the name of the Kopia compression algorithm to use. | True |  | Enum: [none deflate-best-compression deflate-best-speed deflate-default gzip gzip-best-compression gzip-best-speed pgzip pgzip-best-compression pgzip-best-speed s2-better s2-default s2-parallel-4 s2-parallel-8 zstd zstd-better-compression zstd-fastest] <br />Required: \{\} <br /> |
| `minSize` _integer_ | MinSize is the minimum file size, in bytes, to attempt compression for.<br />Files smaller than this are stored uncompressed. Zero means no minimum. |  |  | Minimum: 0 <br />Optional: \{\} <br /> |
| `maxSize` _integer_ | MaxSize is the maximum file size, in bytes, to attempt compression for.<br />Files larger than this are stored uncompressed. Zero means no maximum. |  |  | Minimum: 0 <br />Optional: \{\} <br /> |


#### Data



Data defines the configuration for the data directory.



_Appears in:_
- [Tier1Configuration](#tier1configuration)

| Field | Description | Required | Default | Validation |
| --- | --- | --- | --- | --- |
| `pvcTemplate` _[PersistentVolumeClaimSpec](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.37/#persistentvolumeclaimspec-v1-core)_ | Template to be used to generate the Persistent Volume Claim needed for the data folder,<br />containing base backups and WAL files. | True |  |  |


#### EmbeddedObjectMeta



EmbeddedObjectMeta contains metadata for embedded objects.



_Appears in:_
- [PodTemplateSpec](#podtemplatespec)

| Field | Description | Required | Default | Validation |
| --- | --- | --- | --- | --- |
| `labels` _object (keys:string, values:string)_ |  |  |  | Optional: \{\} <br /> |
| `annotations` _object (keys:string, values:string)_ |  |  |  | Optional: \{\} <br /> |


#### FileReference



FileReference specifies a file from a volume source.



_Appears in:_
- [FileSource](#filesource)

| Field | Description | Required | Default | Validation |
| --- | --- | --- | --- | --- |
| `volume` _[VolumeSource](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.37/#volumesource-v1-core)_ | Volume is the volume source to mount. | True |  |  |
| `path` _string_ | Path is the file path within the mounted volume. | True |  |  |


#### FileSource



FileSource specifies a source for a file. This wrapper allows future
alternatives to be added without breaking the API.

_Validation:_
- ExactlyOneOf: [fileReference]

_Appears in:_
- [Tier1Configuration](#tier1configuration)
- [Tier2Configuration](#tier2configuration)

| Field | Description | Required | Default | Validation |
| --- | --- | --- | --- | --- |
| `fileReference` _[FileReference](#filereference)_ | FileReference specifies a file from a volume source. |  |  | Optional: \{\} <br /> |


#### ImageConfiguration



ImageConfiguration contains the information needed to download
the Klio image.



_Appears in:_
- [ServerSpec](#serverspec)

| Field | Description | Required | Default | Validation |
| --- | --- | --- | --- | --- |
| `image` _string_ | Image is the image to be used for the Klio server | True |  |  |
| `imagePullPolicy` _[PullPolicy](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.37/#pullpolicy-v1-core)_ | ImagePullPolicy defines the policy for pulling the image |  | IfNotPresent | Optional: \{\} <br /> |
| `imagePullSecrets` _[LocalObjectReference](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.37/#localobjectreference-v1-core) array_ | ImagePullSecrets is an optional list of references to secrets in the same namespace to use for pulling any of the<br />images |  |  | Optional: \{\} <br /> |


#### PluginConfiguration



PluginConfiguration is the Schema for the client configuration API.





| Field | Description | Required | Default | Validation |
| --- | --- | --- | --- | --- |
| `apiVersion` _string_ | `klio.cnpg.io/v1alpha1` | True | | |
| `kind` _string_ | `PluginConfiguration` | True | | |
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.37/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. | True |  |  |
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
| `walPrefetch` _[WALPrefetchConfiguration](#walprefetchconfiguration)_ | WALPrefetch configures WAL prefetching behavior during recovery operations. |  |  | Optional: \{\} <br /> |
| `clientSecretName` _string_ | ClientSecretName is the name of the secret containing the client credentials | True |  | MinLength: 1 <br />Required: \{\} <br /> |
| `serverSecretName` _string_ | ServerSecretName is the name of the secret containing the server TLS certificate | True |  | MinLength: 1 <br />Required: \{\} <br /> |
| `clusterName` _string_ | ClusterName is the name of the PostgreSQL cluster we are connecting to | True |  | MinLength: 1 <br />Required: \{\} <br /> |
| `pprof` _boolean_ | Pprof enables the pprof endpoint for performance profiling |  |  | Optional: \{\} <br /> |
| `mode` _[ServerMode](#servermode)_ | Mode selects the operation mode of the plugin. | True | standard | Enum: [standard read-only] <br /> |
| `containers` _[Container](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.37/#container-v1-core) array_ | Containers allows defining a list of containers that will be merged with the Klio sidecar containers.<br />This enables users to customize the sidecars with additional environment variables, volume mounts,<br />resource limits, and other container settings without polluting the PostgreSQL container environment.<br />Merge behavior:<br />- Containers are matched by name (klio-plugin, klio-restore)<br />- User customizations serve as the base<br />- Klio required values (name, args, CONTAINER_NAME env var) always override user values<br />- User-defined environment variables and volume mounts are preserved<br />- Template defaults are applied only for fields not set by the user or Klio |  |  | MaxItems: 2 <br />Optional: \{\} <br /> |


#### PluginConfigurationStatus



PluginConfigurationStatus defines the observed state of ClientConfig.



_Appears in:_
- [PluginConfiguration](#pluginconfiguration)

| Field | Description | Required | Default | Validation |
| --- | --- | --- | --- | --- |
| `conditions` _[Condition](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.37/#condition-v1-meta) array_ | Conditions represent the latest available observations of the<br />PluginConfiguration's state. |  |  | Optional: \{\} <br /> |


#### PodTemplateSpec



PodTemplateSpec describes the data a pod should have when created from a template.



_Appears in:_
- [ServerSpec](#serverspec)

| Field | Description | Required | Default | Validation |
| --- | --- | --- | --- | --- |
| `metadata` _[EmbeddedObjectMeta](#embeddedobjectmeta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  |  | Optional: \{\} <br /> |
| `spec` _[PodSpec](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.37/#podspec-v1-core)_ |  |  |  | Optional: \{\} <br /> |


#### Queue



Queue defines the configuration for the directory hosting the
task queue.



_Appears in:_
- [ServerSpec](#serverspec)

| Field | Description | Required | Default | Validation |
| --- | --- | --- | --- | --- |
| `pvcTemplate` _[PersistentVolumeClaimSpec](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.37/#persistentvolumeclaimspec-v1-core)_ | PersistentVolumeClaimTemplate is used to generate the configuration for<br />the PVC hosting the work queue. | True |  |  |


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
| `prefix` _string_ | Prefix is the path within the bucket under which all Klio objects<br />are stored, allowing a single bucket to be shared across multiple deployments. |  |  | Optional: \{\} <br /> |
| `endpoint` _string_ | Endpoint is the endpoint to be used |  |  | Optional: \{\} <br /> |
| `region` _string_ | Region is the region to be used |  |  | Optional: \{\} <br /> |
| `accessKeyId` _[SecretKeySelector](https://pkg.go.dev/github.com/cloudnative-pg/machinery/pkg/api#SecretKeySelector)_ | The S3 access key ID |  |  | Optional: \{\} <br /> |
| `secretAccessKey` _[SecretKeySelector](https://pkg.go.dev/github.com/cloudnative-pg/machinery/pkg/api#SecretKeySelector)_ | The S3 access key |  |  | Optional: \{\} <br /> |
| `sessionToken` _[SecretKeySelector](https://pkg.go.dev/github.com/cloudnative-pg/machinery/pkg/api#SecretKeySelector)_ | The S3 session token |  |  | Optional: \{\} <br /> |
| `customCaBundle` _[SecretKeySelector](https://pkg.go.dev/github.com/cloudnative-pg/machinery/pkg/api#SecretKeySelector)_ | A pointer to a custom CA bundle |  |  | Optional: \{\} <br /> |


#### Server



Server is the Schema for the servers API.





| Field | Description | Required | Default | Validation |
| --- | --- | --- | --- | --- |
| `apiVersion` _string_ | `klio.cnpg.io/v1alpha1` | True | | |
| `kind` _string_ | `Server` | True | | |
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.37/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. | True |  |  |
| `spec` _[ServerSpec](#serverspec)_ |  | True |  |  |
| `status` _[ServerStatus](#serverstatus)_ |  |  |  | Optional: \{\} <br /> |


#### ServerMode

_Underlying type:_ _string_

ServerMode defines the operation mode of the Server.



_Appears in:_
- [PluginConfigurationSpec](#pluginconfigurationspec)
- [ServerSpec](#serverspec)

| Field | Description |
| --- | --- |
| `standard` | ModeStandard corresponds to server with standard read/write permissions.<br /> |
| `read-only` | ModeReadOnly corresponds to a server with read-only permissions.<br /> |


#### ServerSpec



ServerSpec defines the desired state of Server.



_Appears in:_
- [Server](#server)

| Field | Description | Required | Default | Validation |
| --- | --- | --- | --- | --- |
| `image` _string_ | Image is the image to be used for the Klio server | True |  |  |
| `imagePullPolicy` _[PullPolicy](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.37/#pullpolicy-v1-core)_ | ImagePullPolicy defines the policy for pulling the image |  | IfNotPresent | Optional: \{\} <br /> |
| `imagePullSecrets` _[LocalObjectReference](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.37/#localobjectreference-v1-core) array_ | ImagePullSecrets is an optional list of references to secrets in the same namespace to use for pulling any of the<br />images |  |  | Optional: \{\} <br /> |
| `tlsSecretName` _string_ | TLSSecretName is the name of the Kubernetes secret containing the server-side certificate<br />to be used for the Klio server. | True |  |  |
| `caSecretName` _string_ | ClientCASecretName is the name of the Kubernetes secret containing the CA certificate<br />to be used by the Klio server to validate the users. | True |  |  |
| `mode` _[ServerMode](#servermode)_ | Mode selects the operation mode of the server. | True | standard | Enum: [standard read-only] <br /> |
| `tier1` _[Tier1Configuration](#tier1configuration)_ | Tier1 is the Tier 1 configuration | True |  |  |
| `tier2` _[Tier2Configuration](#tier2configuration)_ | Tier2 is the Tier 2 configuration | True |  |  |
| `queue` _[Queue](#queue)_ | Queue is the configuration of the PVC that should host<br />the task queue. |  |  | Optional: \{\} <br /> |
| `template` _[PodTemplateSpec](#podtemplatespec)_ | Template to override the default StatefulSet of the Klio server.<br />WARNING: Modifying this template may break the server functionality if not done carefully.<br />This field is primarily intended for advanced configuration such as telemetry setup.<br />Use at your own risk and ensure thorough testing before applying changes. |  |  | Optional: \{\} <br /> |


#### ServerStatus



ServerStatus defines the observed state of Server.



_Appears in:_
- [Server](#server)



#### TLSConfiguration



TLSConfiguration contains the information needed to configure
the PKI infrastructure of the Klio server.



_Appears in:_
- [ServerSpec](#serverspec)

| Field | Description | Required | Default | Validation |
| --- | --- | --- | --- | --- |
| `tlsSecretName` _string_ | TLSSecretName is the name of the Kubernetes secret containing the server-side certificate<br />to be used for the Klio server. | True |  |  |
| `caSecretName` _string_ | ClientCASecretName is the name of the Kubernetes secret containing the CA certificate<br />to be used by the Klio server to validate the users. | True |  |  |


#### Tier1Configuration



Tier1Configuration is the tier 1 configuration.



_Appears in:_
- [ServerSpec](#serverspec)

| Field | Description | Required | Default | Validation |
| --- | --- | --- | --- | --- |
| `cache` _[Cache](#cache)_ | Cache is the configuration of the PVC that should be<br />used for the cache. | True |  |  |
| `data` _[Data](#data)_ | Data is the configuration of the PVC that should be used<br />for the base backups. | True |  |  |
| `encryptionKeyFile` _[FileSource](#filesource)_ | EncryptionKeyFile specifies the Age-encrypted encryption key file. | True |  | ExactlyOneOf: [fileReference] <br /> |
| `identityFile` _[FileSource](#filesource)_ | IdentityFile specifies the Age identity (private key) file used to<br />decrypt the encryption key. | True |  | ExactlyOneOf: [fileReference] <br /> |
| `compression` _[CompressionPolicy](#compressionpolicy)_ | Compression defines the repository-wide (global) compression policy<br />applied to base backups stored on tier1. Individual clusters can<br />override it through their PluginConfiguration. |  |  | Optional: \{\} <br /> |


#### Tier1PluginConfiguration



Tier1PluginConfiguration configures tier1 backup and recovery settings.



_Appears in:_
- [PluginConfigurationSpec](#pluginconfigurationspec)

| Field | Description | Required | Default | Validation |
| --- | --- | --- | --- | --- |
| `retention` _[RetentionPolicy](#retentionpolicy)_ | RetentionPolicy defines how many backups we should keep |  |  | Optional: \{\} <br /> |
| `compression` _[CompressionPolicy](#compressionpolicy)_ | Compression defines the compression policy applied to this cluster's<br />base backups on tier1. It overrides the tier1 repository-wide policy<br />configured on the Server. |  |  | Optional: \{\} <br /> |


#### Tier2Configuration



Tier2Configuration is the tier 2 configuration.



_Appears in:_
- [ServerSpec](#serverspec)

| Field | Description | Required | Default | Validation |
| --- | --- | --- | --- | --- |
| `cache` _[Cache](#cache)_ | Cache is the configuration of the PVC that should be<br />used for the cache. | True |  |  |
| `s3` _[S3Configuration](#s3configuration)_ | S3 contains the configuration parameters for an S3-based tier 2. | True |  |  |
| `encryptionKeyFile` _[FileSource](#filesource)_ | EncryptionKeyFile specifies the Age-encrypted encryption key file. | True |  | ExactlyOneOf: [fileReference] <br /> |
| `identityFile` _[FileSource](#filesource)_ | IdentityFile specifies the Age identity (private key) file used to<br />decrypt the encryption key. | True |  | ExactlyOneOf: [fileReference] <br /> |
| `compression` _[CompressionPolicy](#compressionpolicy)_ | Compression defines the repository-wide (global) compression policy<br />applied to base backups stored on tier2. Individual clusters can<br />override it through their PluginConfiguration. |  |  | Optional: \{\} <br /> |


#### Tier2PluginConfiguration



Tier2PluginConfiguration configures tier2 backup and recovery settings.



_Appears in:_
- [PluginConfigurationSpec](#pluginconfigurationspec)

| Field | Description | Required | Default | Validation |
| --- | --- | --- | --- | --- |
| `enableBackup` _boolean_ | EnableBackup controls whether WAL and base backups should be stored in tier2 |  |  | Optional: \{\} <br /> |
| `enableRecovery` _boolean_ | EnableRecovery controls whether tier2 should be included in the recovery source list |  |  | Optional: \{\} <br /> |
| `retention` _[RetentionPolicy](#retentionpolicy)_ | RetentionPolicy defines how many backups we should keep |  |  | Optional: \{\} <br /> |
| `compression` _[CompressionPolicy](#compressionpolicy)_ | Compression defines the compression policy applied to this cluster's<br />base backups on tier2. It overrides the tier2 repository-wide policy<br />configured on the Server. |  |  | Optional: \{\} <br /> |


#### WALPrefetchConfiguration



WALPrefetchConfiguration configures WAL prefetching during recovery.



_Appears in:_
- [PluginConfigurationSpec](#pluginconfigurationspec)

| Field | Description | Required | Default | Validation |
| --- | --- | --- | --- | --- |
| `count` _integer_ | Count is the number of WAL files to prefetch ahead during recovery.<br />A value of 0 disables prefetching. | True | 2 | Maximum: 64 <br />Minimum: 0 <br /> |
| `maxConcurrentDownloads` _integer_ | MaxConcurrentDownloads is the maximum number of concurrent WAL downloads. | True | 4 | Maximum: 64 <br />Minimum: 1 <br /> |


