# Protocol Documentation
<a name="top"></a>

## Table of Contents

- [wal.proto](#wal-proto)
    - [GetLatestRequest](#klio-wal-v1-GetLatestRequest)
    - [GetLatestResult](#klio-wal-v1-GetLatestResult)
    - [GetRequest](#klio-wal-v1-GetRequest)
    - [GetResult](#klio-wal-v1-GetResult)
    - [PutRequest](#klio-wal-v1-PutRequest)
    - [PutResult](#klio-wal-v1-PutResult)
    - [StartWALFile](#klio-wal-v1-StartWALFile)
    - [WALFileBlock](#klio-wal-v1-WALFileBlock)
  
    - [WAL](#klio-wal-v1-WAL)
  
- [Scalar Value Types](#scalar-value-types)



<a name="wal-proto"></a>
<p align="right"><a href="#top">Top</a></p>

## wal.proto



<a name="klio-wal-v1-GetLatestRequest"></a>

### GetLatestRequest



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| cluster_name | [string](#string) |  |  |






<a name="klio-wal-v1-GetLatestResult"></a>

### GetLatestResult



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| wal_name | [string](#string) | optional |  |






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






<a name="klio-wal-v1-StartWALFile"></a>

### StartWALFile



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| klio_version | [uint64](#uint64) |  |  |
| file_length | [uint64](#uint64) |  |  |






<a name="klio-wal-v1-WALFileBlock"></a>

### WALFileBlock



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| range | [bytes](#bytes) |  |  |
| encryption_version | [uint64](#uint64) |  |  |





 

 

 


<a name="klio-wal-v1-WAL"></a>

### WAL


| Method Name | Request Type | Response Type | Description |
| ----------- | ------------ | ------------- | ------------|
| Put | [PutRequest](#klio-wal-v1-PutRequest) stream | [PutResult](#klio-wal-v1-PutResult) |  |
| Get | [GetRequest](#klio-wal-v1-GetRequest) | [GetResult](#klio-wal-v1-GetResult) stream |  |
| GetLatest | [GetLatestRequest](#klio-wal-v1-GetLatestRequest) | [GetLatestResult](#klio-wal-v1-GetLatestResult) |  |

 



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

