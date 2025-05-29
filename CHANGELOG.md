# Changelog

## 0.0.1 (2025-05-29)


### Features

* Backup POC ([#13](https://github.com/EnterpriseDB/klio/issues/13)) ([9be50f8](https://github.com/cloudnative-pg/klio/commit/9be50f8c2c1c1e893013fb516c3bcd1770f05e31))
* Back up control file in a separate step ([#35](https://github.com/EnterpriseDB/klio/issues/35)) ([3e180c3](https://github.com/cloudnative-pg/klio/commit/3e180c355d361b9a1a4c2bb00c60f6a11f653f4f))
* Backup exclusion list ([81b1c2c](https://github.com/cloudnative-pg/klio/commit/81b1c2c9cd9dc7bf382c1bf4223fa0819b05108a))
* Compile packages ([21e5f5e](https://github.com/cloudnative-pg/klio/commit/21e5f5e1c294d96825d1dfd2a91b5703c8d76a78))
* Deploy on Kubernetes, dagger module ([#17](https://github.com/EnterpriseDB/klio/issues/17)) ([14d77f6](https://github.com/cloudnative-pg/klio/commit/14d77f6d8bfbd52c5aca5b62a5106c22955dc202))
* Download history files ([#20](https://github.com/EnterpriseDB/klio/issues/20)) ([80d5509](https://github.com/cloudnative-pg/klio/commit/80d5509e8aea2f55aac3af6d3f885cbd45dcd240))
* Experiment on FIPS compliance ([0a91396](https://github.com/cloudnative-pg/klio/commit/0a9139610da5a606f197eae3d19187677fe49bee))
* Factor out streaming calls ([4f83ac0](https://github.com/cloudnative-pg/klio/commit/4f83ac02dc54cb8d209a28e531f0f7c65661bd35))
* Get WAL benchmark ([#8](https://github.com/EnterpriseDB/klio/issues/8)) ([8bb1c96](https://github.com/cloudnative-pg/klio/commit/8bb1c963ada7fdd79cd84c8350ff6b557e67de10))
* Get-wal ([#16](https://github.com/EnterpriseDB/klio/issues/16)) ([57e25da](https://github.com/cloudnative-pg/klio/commit/57e25da7a9c74029e189d2fda4d551c970e7e849))
* GetWalStreaming ([a8d3c13](https://github.com/cloudnative-pg/klio/commit/a8d3c139c3415568b2ba0ec05f0a36ddf2a59c3b))
* Handle streaming replication CopyDone message ([#23](https://github.com/EnterpriseDB/klio/issues/23)) ([b596b0d](https://github.com/cloudnative-pg/klio/commit/b596b0d868aa1a3b1a8235bda481df931efa0aa0))
* Initial import ([0b07dea](https://github.com/cloudnative-pg/klio/commit/0b07dea9e384b5f4ed1e50905ecbdad9043051e8))
* Isolate WAL receiver ([#7](https://github.com/EnterpriseDB/klio/issues/7)) ([6147a08](https://github.com/cloudnative-pg/klio/commit/6147a080983ae798994c4222f10605944c5d1e2e))
* Keep track of expected WAL size ([14f0d55](https://github.com/cloudnative-pg/klio/commit/14f0d55d1cf6a9060818f1a86cabd9b29f68ca58))
* Klio recover ([4294f85](https://github.com/cloudnative-pg/klio/commit/4294f8518de8e37f42cc3217b9651cd8bc0f40a4))
* Properly calculate latestWAL ([#4](https://github.com/EnterpriseDB/klio/issues/4)) ([fb4cb35](https://github.com/cloudnative-pg/klio/commit/fb4cb35ed1464efa49ff548f9835ad782a9309fc))
* Restart from the latest WAL file ([#22](https://github.com/EnterpriseDB/klio/issues/22)) ([18e1cf9](https://github.com/cloudnative-pg/klio/commit/18e1cf972a27c0232759bb0f9b84289b6a34c683))
* Restructure project packages ([#21](https://github.com/EnterpriseDB/klio/issues/21)) ([24f5b9b](https://github.com/cloudnative-pg/klio/commit/24f5b9b9cddfcd4975febb770f66f061524f9715))
* Support downloading partial files ([3f33b6a](https://github.com/cloudnative-pg/klio/commit/3f33b6a4e09068573c81bb44058c682c04b1aff9))
* Systemd unit files ([ee5d5e0](https://github.com/cloudnative-pg/klio/commit/ee5d5e03da0f72cf8745739598cefc70a9ca668f))
* Tier1 kopia server ([#3](https://github.com/EnterpriseDB/klio/issues/3)) ([4d265f6](https://github.com/cloudnative-pg/klio/commit/4d265f6359e2a84ccb5aa3717603c8dcf509e2a6))
* Use standard DSN for backups ([#18](https://github.com/EnterpriseDB/klio/issues/18)) ([b7e4bf7](https://github.com/cloudnative-pg/klio/commit/b7e4bf7351d5e8423979c4ac82b37a26991449d0))
* WAL repository encryption ([#11](https://github.com/EnterpriseDB/klio/issues/11)) ([69f7a82](https://github.com/cloudnative-pg/klio/commit/69f7a82f62fbdf7ac939c8706d37cb753119a458))
* Wal server ([#10](https://github.com/EnterpriseDB/klio/issues/10)) ([94acc25](https://github.com/cloudnative-pg/klio/commit/94acc25a62a39dc25c24b7dde649dfe629daa932))
* WAL streaming to Klio server ([#12](https://github.com/EnterpriseDB/klio/issues/12)) ([943f548](https://github.com/cloudnative-pg/klio/commit/943f5489be95689fd2df762b89ee2dcb380394b2))


### Bug Fixes

* Remove context.TODO() from the non-test code ([#14](https://github.com/EnterpriseDB/klio/issues/14)) ([496a6fe](https://github.com/cloudnative-pg/klio/commit/496a6fe4a2485a0439b2e5fc098027bba0c8c2c2))
* Add restart directive in systemd services ([cb85f3a](https://github.com/cloudnative-pg/klio/commit/cb85f3a1cced488b2cfc814a467cdd18b395b58a))
* History file archive location ([ee72075](https://github.com/cloudnative-pg/klio/commit/ee72075e0fbf324c8460cfe80e1de88233b362b9))
* Add missing json tag to annotations ([d4a1dcc](https://github.com/cloudnative-pg/klio/commit/d4a1dccde623fcc203e62778f194440f85f8f797))
* Correctly handle missing WAL case ([a507126](https://github.com/cloudnative-pg/klio/commit/a507126f82724187fcd94871ff9d9c9fd19e0ef2))
* Correctly handle PG shutdown ([f93bea6](https://github.com/cloudnative-pg/klio/commit/f93bea670ee1bd2af8d2cd8ce013305f8ae94955))
* Get-wal command line parsing ([7ab5c79](https://github.com/cloudnative-pg/klio/commit/7ab5c79b5ccdcac4f00467f1f1b9588039566b46))
* Get-wal error message when WAL file is not existing ([af0a664](https://github.com/cloudnative-pg/klio/commit/af0a6642b9677f446e6ca0508cdd22ef7272c8e5))
* Replication slot name check ([54761a1](https://github.com/cloudnative-pg/klio/commit/54761a17a8137d0b27e337827fbfda1502f24660))
* Sync .dagger go.mod ([#1](https://github.com/EnterpriseDB/klio/issues/1)) ([de60cf8](https://github.com/cloudnative-pg/klio/commit/de60cf8339bb87752e3855a7982e33c0c5a334f8))
* **tier1:** Correctly initialize repository ([d17129d](https://github.com/cloudnative-pg/klio/commit/d17129dea286866d4cccd97c03dbea88c462459a))
