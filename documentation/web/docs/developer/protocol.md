# Protocol Documentation
<a name="top"></a>

## Table of Contents

- [klio_wal.proto](#klio_wal-proto)
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



<a name="klio_wal-proto"></a>
<p align="right"><a href="#top">Top</a></p>

## klio_wal.proto



<a name="klio-wal-v1-ClusterMetadata"></a>

### ClusterMetadata
The following messages are written in the cluster metadata
file


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| system_id | [string](#string) |  | The system ID of the current cluster |
| gaps | [WALGap](#klio-wal-v1-WALGap) | repeated | The gaps we are aware of in the collected WALs. |






<a name="klio-wal-v1-GetMetadataRequest"></a>

### GetMetadataRequest



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| cluster_name | [string](#string) |  |  |






<a name="klio-wal-v1-GetRequest"></a>

### GetRequest



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| cluster_name | [string](#string) |  |  |
| wal_name | [string](#string) |  |  |






<a name="klio-wal-v1-GetResult"></a>

### GetResult



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| wal_block | [bytes](#bytes) |  |  |
| segment_size | [uint64](#uint64) |  |  |






<a name="klio-wal-v1-PutRequest"></a>

### PutRequest



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| cluster_name | [string](#string) |  |  |
| wal_name | [string](#string) |  |  |
| wal_block | [bytes](#bytes) |  |  |
| segment_size | [uint64](#uint64) |  |  |






<a name="klio-wal-v1-PutResult"></a>

### PutResult



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| written_size | [uint64](#uint64) |  |  |






<a name="klio-wal-v1-RequestWALStartRequest"></a>

### RequestWALStartRequest



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| cluster_name | [string](#string) |  | This is the cluster name |
| system_id | [string](#string) |  | This is the system ID |
| current_wal_name | [string](#string) |  | This is the current WAL name that is being written by PostgreSQL. If empty, the start WAL name will be found by looking at the stored WAL files. |






<a name="klio-wal-v1-RequestWALStartResult"></a>

### RequestWALStartResult



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| wal_name | [string](#string) |  | The WAL file where the client is expected to start streaming. |






<a name="klio-wal-v1-ResetWALStreamRequest"></a>

### ResetWALStreamRequest



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| cluster_name | [string](#string) |  | This is the cluster name |
| system_id | [string](#string) |  | This is the system ID |
| current_wal_name | [string](#string) |  | This is the current WAL name that is being written by PostgreSQL. If empty, the start WAL name will be found by looking at the stored WAL files. |






<a name="klio-wal-v1-ResetWALStreamResult"></a>

### ResetWALStreamResult



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| wal_name | [string](#string) |  | The WAL file where the client is expected to start streaming. |






<a name="klio-wal-v1-StartWALFile"></a>

### StartWALFile



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| klio_version | [uint64](#uint64) |  |  |
| file_length | [uint64](#uint64) |  |  |






<a name="klio-wal-v1-WALGap"></a>

### WALGap
WALGap is a know gap in the WAL collection process.
This is usually caused by an invocation of the reset-lsn Klio
feature.


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| ts | [google.protobuf.Timestamp](#google-protobuf-Timestamp) |  | When this gap was detected and created. |
| start | [string](#string) |  | When the gap started. |
| end | [string](#string) |  | When the gap ends. |





 

 

 


<a name="klio-wal-v1-WAL"></a>

### WAL


| Method Name | Request Type | Response Type | Description |
| ----------- | ------------ | ------------- | ------------|
| Put | [PutRequest](#klio-wal-v1-PutRequest) stream | [PutResult](#klio-wal-v1-PutResult) |  |
| Get | [GetRequest](#klio-wal-v1-GetRequest) | [GetResult](#klio-wal-v1-GetResult) stream |  |
| GetMetadata | [GetMetadataRequest](#klio-wal-v1-GetMetadataRequest) | [ClusterMetadata](#klio-wal-v1-ClusterMetadata) |  |
| RequestWALStart | [RequestWALStartRequest](#klio-wal-v1-RequestWALStartRequest) | [RequestWALStartResult](#klio-wal-v1-RequestWALStartResult) |  |
| ResetWALStream | [ResetWALStreamRequest](#klio-wal-v1-ResetWALStreamRequest) | [ResetWALStreamResult](#klio-wal-v1-ResetWALStreamResult) |  |

 



## Scalar Value Types

| .proto Type | Notes | C++ | Java | Python | Go | C# | PHP | Ruby |
| ----------- | ----- | --- | ---- | ------ | -- | -- | --- | ---- |
| <a name="double" /> double |  | double | double | float | float64 | double | float | Float |
| <a name="float" /> float |  | float | float | float | float32 | float | float | Float |
| <a name="int32" /> int32 | Uses variable-length encoding. Inefficient for encoding negative numbers – if your field is likely to have negative values, use sint32 instead. | int32 | int | int | int32 | int | integer | Bignum or Fixnum (as required) |
| <a name="int64" /> int64 | Uses variable-length encoding. Inefficient for encoding negative numbers – if your field is likely to have negative values, use sint64 instead. | int64 | long | int/long | int64 | long | integer/string | Bignum |
| <a name="uint32" /> uint32 | Uses variable-length encoding. | uint32 | int | int/long | uint32 | uint | integer | Bignum or Fixnum (as required) |
| <a name="uint64" /> uint64 | Uses variable-length encoding. | uint64 | long | int/long | uint64 | ulong | integer/string | Bignum or Fixnum (as required) |
| <a name="sint32" /> sint32 | Uses variable-length encoding. Signed int value. These more efficiently encode negative numbers than regular int32s. | int32 | int | int | int32 | int | integer | Bignum or Fixnum (as required) |
| <a name="sint64" /> sint64 | Uses variable-length encoding. Signed int value. These more efficiently encode negative numbers than regular int64s. | int64 | long | int/long | int64 | long | integer/string | Bignum |
| <a name="fixed32" /> fixed32 | Always four bytes. More efficient than uint32 if values are often greater than 2^28. | uint32 | int | int | uint32 | uint | integer | Bignum or Fixnum (as required) |
| <a name="fixed64" /> fixed64 | Always eight bytes. More efficient than uint64 if values are often greater than 2^56. | uint64 | long | int/long | uint64 | ulong | integer/string | Bignum |
| <a name="sfixed32" /> sfixed32 | Always four bytes. | int32 | int | int | int32 | int | integer | Bignum or Fixnum (as required) |
| <a name="sfixed64" /> sfixed64 | Always eight bytes. | int64 | long | int/long | int64 | long | integer/string | Bignum |
| <a name="bool" /> bool |  | bool | boolean | boolean | bool | bool | boolean | TrueClass/FalseClass |
| <a name="string" /> string | A string must always contain UTF-8 encoded or 7-bit ASCII text. | string | String | str/unicode | string | string | string | String (UTF-8) |
| <a name="bytes" /> bytes | May contain any arbitrary sequence of bytes. | string | ByteString | str | []byte | ByteString | string | String (ASCII-8BIT) |

