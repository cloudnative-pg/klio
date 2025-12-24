## Packages
- [kliocatalog.cnpg.io/v1alpha1](#kliocatalogenterprisedbiov1alpha1)


## kliocatalog.cnpg.io/v1alpha1

Package v1alpha1 the Klio Catalog API

### Resource Types
- [KlioBackup](#kliobackup)
- [KlioBackupList](#kliobackuplist)



#### KlioBackup



KlioBackup is the Schema for a Klio Backup API.



_Appears in:_
- [KlioBackupList](#kliobackuplist)

| Field | Description | Required | Default | Validation |
| --- | --- | --- | --- | --- |
| `apiVersion` _string_ | `kliocatalog.cnpg.io/v1alpha1` | True | | |
| `kind` _string_ | `KlioBackup` | True | | |
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.35/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. | True |  |  |
| `spec` _[KlioBackupSpec](#kliobackupspec)_ |  | True |  |  |
| `status` _[KlioBackupStatus](#kliobackupstatus)_ |  |  |  | Optional: \{\} <br /> |


#### KlioBackupList



KlioBackupList contains a list of KlioBackup.





| Field | Description | Required | Default | Validation |
| --- | --- | --- | --- | --- |
| `apiVersion` _string_ | `kliocatalog.cnpg.io/v1alpha1` | True | | |
| `kind` _string_ | `KlioBackupList` | True | | |
| `metadata` _[ListMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.35/#listmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. | True |  |  |
| `items` _[KlioBackup](#kliobackup) array_ |  | True |  |  |


#### KlioBackupSpec



KlioBackupSpec defines the desired state of a KlioBackup.



_Appears in:_
- [KlioBackup](#kliobackup)

| Field | Description | Required | Default | Validation |
| --- | --- | --- | --- | --- |
| `clusterName` _string_ | ClusterName is the name of the cluster that has been backed up | True |  |  |
| `backupID` _string_ | BackupID is the unique identifier of the backup | True |  |  |


#### KlioBackupStatus



KlioBackupStatus defines the observed state of a KlioBackup.



_Appears in:_
- [KlioBackup](#kliobackup)

| Field | Description | Required | Default | Validation |
| --- | --- | --- | --- | --- |
| `startLSN` _integer_ | StartLSN is the LSN of the backup start | True |  |  |
| `endLSN` _integer_ | EndLSN is the LSN of the backup end | True |  |  |
| `startWAL` _string_ | StartWAL is the current WAL when the backup started | True |  |  |
| `endWAL` _string_ | EndWAL is the current WAL when the backup ends | True |  |  |
| `tablespaces` _[TablespaceLayoutList](#tablespacelayoutlist)_ | Tablespaces are the metadata of the tablespaces | True |  |  |
| `annotations` _object (keys:string, values:string)_ | Annotations is a generic data store where each<br />backend can put its metadata. | True |  |  |
| `startedAt` _[Time](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.35/#time-v1-meta)_ | StartedAt is the current time when the backup started. | True |  |  |
| `stoppedAt` _[Time](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.35/#time-v1-meta)_ | StoppedAt is the current time when the backup ended. | True |  |  |


#### TablespaceLayout



TablespaceLayout is the on-disk structure of a tablespace.



_Appears in:_
- [TablespaceLayoutList](#tablespacelayoutlist)

| Field | Description | Required | Default | Validation |
| --- | --- | --- | --- | --- |
| `name` _string_ | Name is the tablespace name | True |  |  |
| `oid` _string_ | Oid is the OID of the tablespace. | True |  |  |
| `path` _string_ | Path is the path where the tablespace can be found. | True |  |  |
| `annotations` _object (keys:string, values:string)_ | Annotations is a generic data store where each backend<br />can annotate its metadata. | True |  |  |


#### TablespaceLayoutList

_Underlying type:_ _[TablespaceLayout](#tablespacelayout)_

TablespaceLayoutList is a list of TablespaceLayout.



_Appears in:_
- [KlioBackupStatus](#kliobackupstatus)

| Field | Description | Required | Default | Validation |
| --- | --- | --- | --- | --- |
| `name` _string_ | Name is the tablespace name | True |  |  |
| `oid` _string_ | Oid is the OID of the tablespace. | True |  |  |
| `path` _string_ | Path is the path where the tablespace can be found. | True |  |  |
| `annotations` _object (keys:string, values:string)_ | Annotations is a generic data store where each backend<br />can annotate its metadata. | True |  |  |


