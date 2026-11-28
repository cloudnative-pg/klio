# Protocol Documentation

## Table of Contents

- [klio_admin.proto](#klio_admin-proto)
    - [DeleteBackupRequest](#klio-wal-v1-DeleteBackupRequest)
    - [DeleteBackupResponse](#klio-wal-v1-DeleteBackupResponse)
    - [FailedBackup](#klio-wal-v1-FailedBackup)
    - [FailedWAL](#klio-wal-v1-FailedWAL)
    - [ListBackupsRequest](#klio-wal-v1-ListBackupsRequest)
    - [ListBackupsResult](#klio-wal-v1-ListBackupsResult)
    - [QueueListFailedBackupsRequest](#klio-wal-v1-QueueListFailedBackupsRequest)
    - [QueueListFailedBackupsResponse](#klio-wal-v1-QueueListFailedBackupsResponse)
    - [QueueListFailedWALsRequest](#klio-wal-v1-QueueListFailedWALsRequest)
    - [QueueListFailedWALsResponse](#klio-wal-v1-QueueListFailedWALsResponse)
    - [QueueStatusRequest](#klio-wal-v1-QueueStatusRequest)
    - [QueueStatusResponse](#klio-wal-v1-QueueStatusResponse)
    - [RefreshRequest](#klio-wal-v1-RefreshRequest)
    - [RefreshResult](#klio-wal-v1-RefreshResult)
  
    - [Tier](#klio-wal-v1-Tier)
  
    - [Admin](#klio-wal-v1-Admin)
  
- [klio_wal.proto](#klio_wal-proto)
    - [CloseBackupRequest](#klio-wal-v1-CloseBackupRequest)
    - [CloseBackupResult](#klio-wal-v1-CloseBackupResult)
    - [ClusterMetadata](#klio-wal-v1-ClusterMetadata)
    - [GetMetadataRequest](#klio-wal-v1-GetMetadataRequest)
    - [GetRequest](#klio-wal-v1-GetRequest)
    - [GetResult](#klio-wal-v1-GetResult)
    - [PutRequest](#klio-wal-v1-PutRequest)
    - [PutResult](#klio-wal-v1-PutResult)
    - [RequestWALStartRequest](#klio-wal-v1-RequestWALStartRequest)
    - [RequestWALStartResult](#klio-wal-v1-RequestWALStartResult)
    - [ResetWALStreamRequest](#klio-wal-v1-ResetWALStreamRequest)
    - [ResetWALStreamResult](#klio-wal-v1-ResetWALStreamResult)
    - [StartWALFile](#klio-wal-v1-StartWALFile)
    - [WALGap](#klio-wal-v1-WALGap)
  
    - [WAL](#klio-wal-v1-WAL)
  
- [Scalar Value Types](#scalar-value-types)




## klio_admin.proto {#klio_admin-proto}



### DeleteBackupRequest {#klio-wal-v1-DeleteBackupRequest}
DeleteBackupRequest is the request to delete a backup from the server.


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| backup_name | [string](#scalar-value-types) |  | The name of the backup to delete. |
| tiers | [Tier](#klio-wal-v1-Tier) | repeated | The storage tiers from which to delete the backup. At least one tier must be specified. |
| cluster_name | [string](#scalar-value-types) |  | The name of the cluster that owns the backup. |






### DeleteBackupResponse {#klio-wal-v1-DeleteBackupResponse}
DeleteBackupResponse is the response to a backup deletion request.






### FailedBackup {#klio-wal-v1-FailedBackup}



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| cluster_name | [string](#scalar-value-types) |  |  |
| last_attempt_time | [google.protobuf.Timestamp](https://protobuf.dev/reference/protobuf/google.protobuf/#timestamp) |  |  |






### FailedWAL {#klio-wal-v1-FailedWAL}



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| cluster_name | [string](#scalar-value-types) |  |  |
| wal_name | [string](#scalar-value-types) |  |  |
| sequence | [uint64](#scalar-value-types) |  |  |
| last_attempt_time | [google.protobuf.Timestamp](https://protobuf.dev/reference/protobuf/google.protobuf/#timestamp) |  |  |






### ListBackupsRequest {#klio-wal-v1-ListBackupsRequest}







### ListBackupsResult {#klio-wal-v1-ListBackupsResult}



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| backup_manifests | [bytes](#scalar-value-types) |  | JSON-serialized array of backup manifests. Each manifest contains fields like: - id: string - cluster_name: string - timestamp: RFC3339 string - size_bytes: number See klioclient.BackupManifest for the canonical structure. We use JSON bytes here to avoid duplicating the internal type definition and conversion logic, as this is a local admin API. |






### QueueListFailedBackupsRequest {#klio-wal-v1-QueueListFailedBackupsRequest}



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| cluster_name | [string](#scalar-value-types) | optional |  |






### QueueListFailedBackupsResponse {#klio-wal-v1-QueueListFailedBackupsResponse}



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| backups | [FailedBackup](#klio-wal-v1-FailedBackup) | repeated |  |






### QueueListFailedWALsRequest {#klio-wal-v1-QueueListFailedWALsRequest}



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| cluster_name | [string](#scalar-value-types) | optional |  |






### QueueListFailedWALsResponse {#klio-wal-v1-QueueListFailedWALsResponse}



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| wals | [FailedWAL](#klio-wal-v1-FailedWAL) | repeated |  |






### QueueStatusRequest {#klio-wal-v1-QueueStatusRequest}







### QueueStatusResponse {#klio-wal-v1-QueueStatusResponse}



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| pending_backups | [uint64](#scalar-value-types) |  | Number of backup synchronization tasks pending in the queue |
| pending_wals | [uint64](#scalar-value-types) |  | Number of WAL relay tasks pending in the queue |






### RefreshRequest {#klio-wal-v1-RefreshRequest}







### RefreshResult {#klio-wal-v1-RefreshResult}









### Tier {#klio-wal-v1-Tier}
Tier represents a storage tier in the backup system.

| Name | Number | Description |
| ---- | ------ | ----------- |
| TIER_UNSPECIFIED | 0 | TIER_UNSPECIFIED is the default value and should not be used. |
| TIER_1 | 1 | TIER_1 represents the local cache storage. |
| TIER_2 | 2 | TIER_2 represents the object storage. |







### Admin {#klio-wal-v1-Admin}


| Method Name | Request Type | Response Type | Description |
| ----------- | ------------ | ------------- | ------------|
| Refresh | [RefreshRequest](#klio-wal-v1-RefreshRequest) | [RefreshResult](#klio-wal-v1-RefreshResult) | Invoked to refresh the policies and the cache of the Kopia server |
| ListBackups | [ListBackupsRequest](#klio-wal-v1-ListBackupsRequest) | [ListBackupsResult](#klio-wal-v1-ListBackupsResult) | List every backup on the server |
| QueueListFailedBackups | [QueueListFailedBackupsRequest](#klio-wal-v1-QueueListFailedBackupsRequest) | [QueueListFailedBackupsResponse](#klio-wal-v1-QueueListFailedBackupsResponse) | List backups failed to be processed from the queue |
| QueueListFailedWALs | [QueueListFailedWALsRequest](#klio-wal-v1-QueueListFailedWALsRequest) | [QueueListFailedWALsResponse](#klio-wal-v1-QueueListFailedWALsResponse) | List WAL files failed to be processed from the queue |
| QueueStatus | [QueueStatusRequest](#klio-wal-v1-QueueStatusRequest) | [QueueStatusResponse](#klio-wal-v1-QueueStatusResponse) | Get the status of the task queue (pending backups and WALs) |
| DeleteBackup | [DeleteBackupRequest](#klio-wal-v1-DeleteBackupRequest) | [DeleteBackupResponse](#klio-wal-v1-DeleteBackupResponse) | Delete a backup from the server |






## klio_wal.proto {#klio_wal-proto}



### CloseBackupRequest {#klio-wal-v1-CloseBackupRequest}
This is sent to the WAL server every time a backup has
been completed.


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| cluster_name | [string](#scalar-value-types) |  | The name of the cluster. |
| backup_name | [string](#scalar-value-types) |  | The name of the backup. |
| timeline | [int32](#scalar-value-types) |  | The backup timeline |
| start_wal | [string](#scalar-value-types) |  | the first WAL required to restore this backup. |
| end_wal | [string](#scalar-value-types) |  | The last WAL required to restore this backup. |
| segment_size | [uint64](#scalar-value-types) |  | The size of a WAL segment. Needed to generate the sequence of WAL files between the start and the end. |
| send_to_tier2 | [bool](#scalar-value-types) |  | Require this backup to be sent to tier2. |
| tier2_retention_policy | [string](#scalar-value-types) |  | When present, set the tier2 retention policy to the specified JSON-serialized policy. |






### CloseBackupResult {#klio-wal-v1-CloseBackupResult}
This is sent by the WAL server in response to a CloseBackupRequest
message.


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| tier2_schedule | [bool](#scalar-value-types) |  | True when the backup has been scheduled to be synchronized to tier2 storage |
| missing_wal_files | [string](#scalar-value-types) | repeated | List of WAL files needed by this backup but still not uploaded to tier1 |






### ClusterMetadata {#klio-wal-v1-ClusterMetadata}
The following messages are written in the cluster metadata
file


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| system_id | [string](#scalar-value-types) |  | The system ID of the current cluster |
| gaps | [WALGap](#klio-wal-v1-WALGap) | repeated | The gaps we are aware of in the collected WALs. |






### GetMetadataRequest {#klio-wal-v1-GetMetadataRequest}



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| cluster_name | [string](#scalar-value-types) |  |  |






### GetRequest {#klio-wal-v1-GetRequest}



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| cluster_name | [string](#scalar-value-types) |  |  |
| wal_name | [string](#scalar-value-types) |  |  |






### GetResult {#klio-wal-v1-GetResult}



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| wal_block | [bytes](#scalar-value-types) |  |  |
| segment_size | [uint64](#scalar-value-types) |  |  |






### PutRequest {#klio-wal-v1-PutRequest}



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| cluster_name | [string](#scalar-value-types) |  |  |
| wal_name | [string](#scalar-value-types) |  |  |
| wal_block | [bytes](#scalar-value-types) |  |  |
| segment_size | [uint64](#scalar-value-types) |  |  |
| send_to_tier2 | [bool](#scalar-value-types) |  |  |






### PutResult {#klio-wal-v1-PutResult}



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| written_size | [uint64](#scalar-value-types) |  |  |






### RequestWALStartRequest {#klio-wal-v1-RequestWALStartRequest}



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| cluster_name | [string](#scalar-value-types) |  | This is the cluster name |
| system_id | [string](#scalar-value-types) |  | This is the system ID |
| current_wal_name | [string](#scalar-value-types) |  | This is the current WAL name that is being written by PostgreSQL. If empty, the start WAL name will be found by looking at the stored WAL files. |






### RequestWALStartResult {#klio-wal-v1-RequestWALStartResult}



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| wal_name | [string](#scalar-value-types) |  | The WAL file where the client is expected to start streaming. |






### ResetWALStreamRequest {#klio-wal-v1-ResetWALStreamRequest}



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| cluster_name | [string](#scalar-value-types) |  | This is the cluster name |
| system_id | [string](#scalar-value-types) |  | This is the system ID |
| current_wal_name | [string](#scalar-value-types) |  | This is the current WAL name that is being written by PostgreSQL. If empty, the start WAL name will be found by looking at the stored WAL files. |






### ResetWALStreamResult {#klio-wal-v1-ResetWALStreamResult}



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| wal_name | [string](#scalar-value-types) |  | The WAL file where the client is expected to start streaming. |






### StartWALFile {#klio-wal-v1-StartWALFile}
The following messages are used to write a WAL file
in the Klio WAL Storage area


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| klio_version | [uint64](#scalar-value-types) |  |  |
| file_length | [uint64](#scalar-value-types) |  |  |






### WALGap {#klio-wal-v1-WALGap}
WALGap is a know gap in the WAL collection process.
This is usually caused by an invocation of the reset-lsn Klio
feature.


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| ts | [google.protobuf.Timestamp](https://protobuf.dev/reference/protobuf/google.protobuf/#timestamp) |  | When this gap was detected and created. |
| start | [string](#scalar-value-types) |  | When the gap started. |
| end | [string](#scalar-value-types) |  | When the gap ends. |












### WAL {#klio-wal-v1-WAL}


| Method Name | Request Type | Response Type | Description |
| ----------- | ------------ | ------------- | ------------|
| Put | [PutRequest](#klio-wal-v1-PutRequest) stream | [PutResult](#klio-wal-v1-PutResult) |  |
| Get | [GetRequest](#klio-wal-v1-GetRequest) | [GetResult](#klio-wal-v1-GetResult) stream |  |
| GetMetadata | [GetMetadataRequest](#klio-wal-v1-GetMetadataRequest) | [ClusterMetadata](#klio-wal-v1-ClusterMetadata) |  |
| RequestWALStart | [RequestWALStartRequest](#klio-wal-v1-RequestWALStartRequest) | [RequestWALStartResult](#klio-wal-v1-RequestWALStartResult) |  |
| ResetWALStream | [ResetWALStreamRequest](#klio-wal-v1-ResetWALStreamRequest) | [ResetWALStreamResult](#klio-wal-v1-ResetWALStreamResult) |  |
| CloseBackup | [CloseBackupRequest](#klio-wal-v1-CloseBackupRequest) | [CloseBackupResult](#klio-wal-v1-CloseBackupResult) |  |





## Scalar Value Types

| .proto Type | Notes | C++ | Java | Python | Go | C# | PHP | Ruby |
| ----------- | ----- | --- | ---- | ------ | -- | -- | --- | ---- |
| double |  | double | double | float | float64 | double | float | Float |
| float |  | float | float | float | float32 | float | float | Float |
| int32 | Uses variable-length encoding. Inefficient for encoding negative numbers – if your field is likely to have negative values, use sint32 instead. | int32 | int | int | int32 | int | integer | Bignum or Fixnum (as required) |
| int64 | Uses variable-length encoding. Inefficient for encoding negative numbers – if your field is likely to have negative values, use sint64 instead. | int64 | long | int/long | int64 | long | integer/string | Bignum |
| uint32 | Uses variable-length encoding. | uint32 | int | int/long | uint32 | uint | integer | Bignum or Fixnum (as required) |
| uint64 | Uses variable-length encoding. | uint64 | long | int/long | uint64 | ulong | integer/string | Bignum or Fixnum (as required) |
| sint32 | Uses variable-length encoding. Signed int value. These more efficiently encode negative numbers than regular int32s. | int32 | int | int | int32 | int | integer | Bignum or Fixnum (as required) |
| sint64 | Uses variable-length encoding. Signed int value. These more efficiently encode negative numbers than regular int64s. | int64 | long | int/long | int64 | long | integer/string | Bignum |
| fixed32 | Always four bytes. More efficient than uint32 if values are often greater than 2^28. | uint32 | int | int | uint32 | uint | integer | Bignum or Fixnum (as required) |
| fixed64 | Always eight bytes. More efficient than uint64 if values are often greater than 2^56. | uint64 | long | int/long | uint64 | ulong | integer/string | Bignum |
| sfixed32 | Always four bytes. | int32 | int | int | int32 | int | integer | Bignum or Fixnum (as required) |
| sfixed64 | Always eight bytes. | int64 | long | int/long | int64 | long | integer/string | Bignum |
| bool |  | bool | boolean | boolean | bool | bool | boolean | TrueClass/FalseClass |
| string | A string must always contain UTF-8 encoded or 7-bit ASCII text. | string | String | str/unicode | string | string | string | String (UTF-8) |
| bytes | May contain any arbitrary sequence of bytes. | string | ByteString | str | []byte | ByteString | string | String (ASCII-8BIT) |

