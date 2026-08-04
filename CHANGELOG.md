# Changelog

## [0.0.18](https://github.com/cloudnative-pg/klio/compare/v0.0.17...v0.0.18) (2026-08-04)


### Features

* **core:** Export Kopia traces via OTLP/gRPC (EnterpriseDB/klio[#1679](https://github.com/cloudnative-pg/klio/issues/1679)) ([245da93](https://github.com/cloudnative-pg/klio/commit/245da93ebdc24d0f301ceaa26134d143ac1cc732))
* **ip:** Contribute Klio to CloudNativePG under the Apache License 2.0 (EnterpriseDB/klio[#1841](https://github.com/cloudnative-pg/klio/issues/1841)) ([a3dcd13](https://github.com/cloudnative-pg/klio/commit/a3dcd1377934dbbe6f025347488aa8c5918c41bd))
* **observability:** Fill Grafana dashboard gaps for WAL and backup metrics (EnterpriseDB/klio[#1793](https://github.com/cloudnative-pg/klio/issues/1793)) ([c88d2f8](https://github.com/cloudnative-pg/klio/commit/c88d2f8fe1ee3b59360c83da0900667907f09694))
* **operator:** Add preflight check operator certification (EnterpriseDB/klio[#1767](https://github.com/cloudnative-pg/klio/issues/1767)) ([295ea20](https://github.com/cloudnative-pg/klio/commit/295ea203c1e3dd9e9f1dc9b942944a47ace91528))


### Bug Fixes

* **ci:** Upload openshift e2e logs from the correct path (EnterpriseDB/klio[#1808](https://github.com/cloudnative-pg/klio/issues/1808)) ([e2fb58e](https://github.com/cloudnative-pg/klio/commit/e2fb58ef0d4dcc7ed3c67ac8c972d9770366cc90))
* **core:** Archive partial WAL segments to tier2 and restore from them (EnterpriseDB/klio[#1747](https://github.com/cloudnative-pg/klio/issues/1747)) ([924914e](https://github.com/cloudnative-pg/klio/commit/924914e2df1ac73650ad455da32bd7e1a605a887))
* **deps:** Pin jsm.go to main for StreamPager cross-delivery fix (EnterpriseDB/klio[#1806](https://github.com/cloudnative-pg/klio/issues/1806)) ([ae18c70](https://github.com/cloudnative-pg/klio/commit/ae18c701ce755b5d07e9379e113ef3459af9c80b))
* **deps:** Update all non-major go dependencies (EnterpriseDB/klio[#1780](https://github.com/cloudnative-pg/klio/issues/1780)) ([c76337c](https://github.com/cloudnative-pg/klio/commit/c76337c45833d14b743fa9967bfd9fbf72409bad))
* **deps:** Update all non-major go dependencies (EnterpriseDB/klio[#1797](https://github.com/cloudnative-pg/klio/issues/1797)) ([21c34a4](https://github.com/cloudnative-pg/klio/commit/21c34a4727e29efc45ea901a0cc0c3ca4f331008))
* **deps:** Update all non-major go dependencies (EnterpriseDB/klio[#1833](https://github.com/cloudnative-pg/klio/issues/1833)) ([8bbd83f](https://github.com/cloudnative-pg/klio/commit/8bbd83fbf5e1c29983902a0f04a91459458caceb))
* **deps:** Update all non-major go dependencies (EnterpriseDB/klio[#1845](https://github.com/cloudnative-pg/klio/issues/1845)) ([762f70d](https://github.com/cloudnative-pg/klio/commit/762f70d20d2124db797e117003c31b66f21fcb8d))
* **deps:** Update all non-major go dependencies (EnterpriseDB/klio[#1848](https://github.com/cloudnative-pg/klio/issues/1848)) ([ef9330e](https://github.com/cloudnative-pg/klio/commit/ef9330e8602c567b3feee4acc4f4551f57ee6e97))
* **deps:** Update documentation dependencies to v3.10.2 (EnterpriseDB/klio[#1798](https://github.com/cloudnative-pg/klio/issues/1798)) ([14c4161](https://github.com/cloudnative-pg/klio/commit/14c4161aec7d10fe4585e8e5555e3928f0cc4fbf))
* **deps:** Update github.com/cloudnative-pg/cloudnative-pg/tests digest to 0552b9c (EnterpriseDB/klio[#1859](https://github.com/cloudnative-pg/klio/issues/1859)) ([e01f8d6](https://github.com/cloudnative-pg/klio/commit/e01f8d612dd2462055b05a38671db1304418955d))
* **deps:** Update github.com/cloudnative-pg/cloudnative-pg/tests digest to 1a79065 (EnterpriseDB/klio[#1803](https://github.com/cloudnative-pg/klio/issues/1803)) ([42c7f9c](https://github.com/cloudnative-pg/klio/commit/42c7f9c8135ed3ff31fa9318347b06906229ed02))
* **deps:** Update github.com/cloudnative-pg/cloudnative-pg/tests digest to 2af6cc5 (EnterpriseDB/klio[#1842](https://github.com/cloudnative-pg/klio/issues/1842)) ([a8c97ed](https://github.com/cloudnative-pg/klio/commit/a8c97eda63eefe671bda9b74cb837c2e96044415))
* **deps:** Update github.com/cloudnative-pg/cloudnative-pg/tests digest to 308f3ea (EnterpriseDB/klio[#1794](https://github.com/cloudnative-pg/klio/issues/1794)) ([2d7ba7b](https://github.com/cloudnative-pg/klio/commit/2d7ba7b0a0be24ec7a58d7a82c05610228c498dc))
* **deps:** Update github.com/cloudnative-pg/cloudnative-pg/tests digest to 83bfc38 (EnterpriseDB/klio[#1865](https://github.com/cloudnative-pg/klio/issues/1865)) ([e418675](https://github.com/cloudnative-pg/klio/commit/e4186751dae758e45e898802da90ad28e5f76648))
* **deps:** Update github.com/cloudnative-pg/cloudnative-pg/tests digest to 91c9e02 (EnterpriseDB/klio[#1810](https://github.com/cloudnative-pg/klio/issues/1810)) ([a5c8de1](https://github.com/cloudnative-pg/klio/commit/a5c8de1dac06f2daefeac8416aa323e1b2904e9c))
* **deps:** Update github.com/cloudnative-pg/cloudnative-pg/tests digest to e39ec6b (EnterpriseDB/klio[#1822](https://github.com/cloudnative-pg/klio/issues/1822)) ([9238238](https://github.com/cloudnative-pg/klio/commit/92382385048336e7de2578a1673fc4e964d5d124))
* **deps:** Update kubernetes monorepo to v0.36.3 (EnterpriseDB/klio[#1854](https://github.com/cloudnative-pg/klio/issues/1854)) ([4189cfe](https://github.com/cloudnative-pg/klio/commit/4189cfeb9925a0ed3619d268c720132d5f96e5a5))
* **deps:** Update module github.com/aws/aws-sdk-go-v2/service/s3 to v1.105.2 (EnterpriseDB/klio[#1816](https://github.com/cloudnative-pg/klio/issues/1816)) ([d66d8e1](https://github.com/cloudnative-pg/klio/commit/d66d8e1c60cce79d5394b688a93b5d80668d7ef3))
* **deps:** Update module go.yaml.in/yaml/v3 to v3.0.5 (EnterpriseDB/klio[#1874](https://github.com/cloudnative-pg/klio/issues/1874)) ([2415112](https://github.com/cloudnative-pg/klio/commit/241511233e75462bbcde935cd69d46701635903e))
* **deps:** Update module google.golang.org/grpc to v1.82.1 (EnterpriseDB/klio[#1813](https://github.com/cloudnative-pg/klio/issues/1813)) ([7a8df3c](https://github.com/cloudnative-pg/klio/commit/7a8df3cc948d3a439115ba8d82aec9e734b20c3c))
* **wal:** Archive timeline history files to tier-2 (EnterpriseDB/klio[#1762](https://github.com/cloudnative-pg/klio/issues/1762)) ([20afc6f](https://github.com/cloudnative-pg/klio/commit/20afc6f09b34b7feb7aa17f9d96c91b0a8586076))

## [0.0.17](https://github.com/EnterpriseDB/klio/compare/v0.0.16...v0.0.17) (2026-07-10)


### Features

* Add nats dead-letter queues ([#1639](https://github.com/EnterpriseDB/klio/issues/1639)) ([14285e1](https://github.com/cloudnative-pg/klio/commit/14285e1e0690d7b3883ea6b263abfe6d4e8bc831))
* Expose DLQ messages via CLI ([#1666](https://github.com/EnterpriseDB/klio/issues/1666)) ([7fbd0ea](https://github.com/cloudnative-pg/klio/commit/7fbd0eac9b36487ffef2ef5ae3f9abbd01521950))
* Expose failed queue tasks via admin CLI and gRPC ([7fbd0ea](https://github.com/cloudnative-pg/klio/commit/7fbd0eac9b36487ffef2ef5ae3f9abbd01521950))
* **metrics:** Add backup duration histogram ([#1589](https://github.com/EnterpriseDB/klio/issues/1589)) ([5d1173c](https://github.com/cloudnative-pg/klio/commit/5d1173cf76a774f3a355c0ba040616b4be81264b))
* **metrics:** Add PostgreSQL backup metrics and use timestamps for snapshots ([#1680](https://github.com/EnterpriseDB/klio/issues/1680)) ([6f141d3](https://github.com/cloudnative-pg/klio/commit/6f141d3553bd9a6976ba890713666f82be9a1a33))
* **metrics:** Rework WAL OpenTelemetry metrics and tracing ([#1703](https://github.com/EnterpriseDB/klio/issues/1703)) ([7e795e7](https://github.com/cloudnative-pg/klio/commit/7e795e77298b4ac6b81558488547ac5598c51943))
* **nats:** Clean up DLQ entries on offload success ([#1663](https://github.com/EnterpriseDB/klio/issues/1663)) ([21fd947](https://github.com/cloudnative-pg/klio/commit/21fd9472a2922a7d1b25bb71fb80a035827ecd76))
* **observability:** Add Grafana dashboard for Klio metrics ([#1708](https://github.com/EnterpriseDB/klio/issues/1708)) ([bd0d712](https://github.com/cloudnative-pg/klio/commit/bd0d7120830d792b11e64b105f89e80d284dd4ce))
* **olm:** Improve the CSV for the operator on OpenShift ([#1333](https://github.com/EnterpriseDB/klio/issues/1333)) ([47afda9](https://github.com/cloudnative-pg/klio/commit/47afda949f3f7dd3003c5d811db8e16397e4ce4f))
* **otel:** Classify backup failures by category ([#1586](https://github.com/EnterpriseDB/klio/issues/1586)) ([a32c40e](https://github.com/cloudnative-pg/klio/commit/a32c40e7c7a9585a19e769ef230f9bae350bf451))
* **otel:** Remove redundant klio.plugin.backup.verifications metric ([#1665](https://github.com/EnterpriseDB/klio/issues/1665)) ([9fcb3a4](https://github.com/cloudnative-pg/klio/commit/9fcb3a4af5dfc7a4a4df749992921bca1df3307d))
* **queue:** Bound retries on WAL and backup consumers ([#1574](https://github.com/EnterpriseDB/klio/issues/1574)) ([a531719](https://github.com/cloudnative-pg/klio/commit/a53171950d6d050a74d84a9dd966bdd789e86911))


### Bug Fixes

* **deps:** Update all non-major go dependencies ([#1602](https://github.com/EnterpriseDB/klio/issues/1602)) ([b085097](https://github.com/cloudnative-pg/klio/commit/b085097e53447bc9d276c7e5d067fe53730bcdc4))
* **deps:** Update all non-major go dependencies ([#1620](https://github.com/EnterpriseDB/klio/issues/1620)) ([4119de7](https://github.com/cloudnative-pg/klio/commit/4119de7e7f94495f216284b317135ab8b4ec655f))
* **deps:** Update all non-major go dependencies ([#1635](https://github.com/EnterpriseDB/klio/issues/1635)) ([49651ef](https://github.com/cloudnative-pg/klio/commit/49651efa01db293ac9f994b6154026a4d42c60de))
* **deps:** Update all non-major go dependencies ([#1652](https://github.com/EnterpriseDB/klio/issues/1652)) ([8e59c13](https://github.com/cloudnative-pg/klio/commit/8e59c1333ef2fe7ed3467d0a4c098b672c172efa))
* **deps:** Update all non-major go dependencies ([#1662](https://github.com/EnterpriseDB/klio/issues/1662)) ([54233f8](https://github.com/cloudnative-pg/klio/commit/54233f85660466411fffc930c02a159cf0f2b6fa))
* **deps:** Update all non-major go dependencies ([#1686](https://github.com/EnterpriseDB/klio/issues/1686)) ([b905fbc](https://github.com/cloudnative-pg/klio/commit/b905fbc5ec5c7e09d2bacf6af016f184bd1ae99a))
* **deps:** Update all non-major go dependencies ([#1724](https://github.com/EnterpriseDB/klio/issues/1724)) ([29f7b61](https://github.com/cloudnative-pg/klio/commit/29f7b6167b7b0e59c2cb273279e5d9a8711fc4f2))
* **deps:** Update all non-major go dependencies ([#1730](https://github.com/EnterpriseDB/klio/issues/1730)) ([e7818b3](https://github.com/cloudnative-pg/klio/commit/e7818b34a025841107418f3be7efebb1d2363690))
* **deps:** Update all non-major go dependencies ([#1734](https://github.com/EnterpriseDB/klio/issues/1734)) ([d2c25ab](https://github.com/cloudnative-pg/klio/commit/d2c25ab302d463d52b467c39be15564bbf702a1f))
* **deps:** Update all non-major go dependencies ([#1758](https://github.com/EnterpriseDB/klio/issues/1758)) ([6894234](https://github.com/cloudnative-pg/klio/commit/6894234c042f7bfa87a7bc70c112eb2e899ad82a))
* **deps:** Update all non-major go dependencies ([#1770](https://github.com/EnterpriseDB/klio/issues/1770)) ([86a92ba](https://github.com/cloudnative-pg/klio/commit/86a92baba4dcd072b533b790bd0f911838c5f717))
* **deps:** Update all non-major go dependencies to v2.30.0 ([#1648](https://github.com/EnterpriseDB/klio/issues/1648)) ([fe81198](https://github.com/cloudnative-pg/klio/commit/fe81198a76d39b5e92a7cf992b8090f28d576a15))
* **deps:** Update github.com/cloudnative-pg/cloudnative-pg/tests digest to 08ead64 ([#1765](https://github.com/EnterpriseDB/klio/issues/1765)) ([0f40919](https://github.com/cloudnative-pg/klio/commit/0f4091931ba73bbd2cfdab39f136b42b18f6064c))
* **deps:** Update github.com/cloudnative-pg/cloudnative-pg/tests digest to 66039df ([#1740](https://github.com/EnterpriseDB/klio/issues/1740)) ([3288673](https://github.com/cloudnative-pg/klio/commit/3288673a367c3fc848eaa9899b9f72cc7c92d0bb))
* **deps:** Update github.com/cloudnative-pg/cloudnative-pg/tests digest to a203777 ([#1750](https://github.com/EnterpriseDB/klio/issues/1750)) ([268c3e2](https://github.com/cloudnative-pg/klio/commit/268c3e24bcaf79755158538a6dd4c452cbecaf35))
* **deps:** Update kubernetes monorepo to v0.36.2 ([#1645](https://github.com/EnterpriseDB/klio/issues/1645)) ([6cd4726](https://github.com/cloudnative-pg/klio/commit/6cd47267092d39b4bdc20189a55f593fcf0958f1))
* **deps:** Update module github.com/cert-manager/cert-manager to v1.20.3 ([#1711](https://github.com/EnterpriseDB/klio/issues/1711)) ([7c82afd](https://github.com/cloudnative-pg/klio/commit/7c82afde39c796f0f3b734c4c5012f3ca5e26464))
* **deps:** Update module github.com/fclairamb/afero-s3 to v0.5.0 ([#1753](https://github.com/EnterpriseDB/klio/issues/1753)) ([6988e52](https://github.com/cloudnative-pg/klio/commit/6988e521e0c73d08d9876d4f5c154a8ef00429cc))
* **deps:** Update module github.com/onsi/gomega to v1.42.1 ([#1699](https://github.com/EnterpriseDB/klio/issues/1699)) ([989c9d3](https://github.com/cloudnative-pg/klio/commit/989c9d343fc6462e0557b2e9be27849168f072c1))
* **queue:** Drain DLQ pager before resolving source messages ([#1714](https://github.com/EnterpriseDB/klio/issues/1714)) ([a463dbe](https://github.com/cloudnative-pg/klio/commit/a463dbe871539cfef7d5fb49230fee2c82bdb533))
* **recovery:** Gate tier2 as a base recovery source on enableRecovery ([#1761](https://github.com/EnterpriseDB/klio/issues/1761)) ([1786da6](https://github.com/cloudnative-pg/klio/commit/1786da69d0652bf82fe4cbe5f1d13dc0ed145434))
* **server:** Refresh tier1 Kopia server after the unpin direct write ([#1769](https://github.com/EnterpriseDB/klio/issues/1769)) ([b41dc27](https://github.com/cloudnative-pg/klio/commit/b41dc27ee8df31113514e3316913d034ee860348))
* **walserver:** Skip latest_written_* metrics for non-segment WAL files ([#1640](https://github.com/EnterpriseDB/klio/issues/1640)) ([aeec3e3](https://github.com/cloudnative-pg/klio/commit/aeec3e3254e03530796636133ebda0339125bfac))

## [0.0.16](https://github.com/EnterpriseDB/klio/compare/v0.0.15...v0.0.16) (2026-06-04)


### ⚠ BREAKING CHANGES

* **otel:** several plugin and server metrics are renamed, collapsed, or change instrument kind; the `tier` attribute value space  becomes `tier1` / `tier2`; and the `clusterName` / `walName` span attribute keys become snake_case. See the migration table in documentation/web/docs/user/opentelemetry.md and the per-change guidance in documentation/web/docs/user/upgrade_notes.md.

### Features

* Add latest written LSN metrics for tier 1 and tier 2 WAL archiving ([#1477](https://github.com/EnterpriseDB/klio/issues/1477)) ([3ee57a5](https://github.com/cloudnative-pg/klio/commit/3ee57a5e3e9fa36bc652fb8c991a072940a0fff6))
* **api:** Make Server and PluginConfiguration mode immutable ([#1554](https://github.com/EnterpriseDB/klio/issues/1554)) ([f688877](https://github.com/cloudnative-pg/klio/commit/f6888776ecd720a310d8e4d1c4995f6dcf826005))
* Detect and report disk-full errors during backup and WAL operations ([#1476](https://github.com/EnterpriseDB/klio/issues/1476)) ([745f41b](https://github.com/cloudnative-pg/klio/commit/745f41b8a2843d31749e87e469183bf0dfa838cd))
* **operator:** Emit metrics via OpenTelemetry bridge ([#1567](https://github.com/EnterpriseDB/klio/issues/1567)) ([e082ab9](https://github.com/cloudnative-pg/klio/commit/e082ab9c85d151334207898a0dfd919504a6cd76))
* **otel:** Label snapshot metrics with tier attribute ([#1555](https://github.com/EnterpriseDB/klio/issues/1555)) ([776fb34](https://github.com/cloudnative-pg/klio/commit/776fb347dbd1a40842babdcc5c60dc4c99b72209))


### Bug Fixes

* Decouple WAL restore tier selection from archive config ([#1428](https://github.com/EnterpriseDB/klio/issues/1428)) ([848971d](https://github.com/cloudnative-pg/klio/commit/848971dd2979d7e594e7b8c9eadf15dc13cf9601))
* **deps:** Update all non-major go dependencies ([#1457](https://github.com/EnterpriseDB/klio/issues/1457)) ([f9ad52d](https://github.com/cloudnative-pg/klio/commit/f9ad52ddfe15d258436e40b21027260f8381981a))
* **deps:** Update all non-major go dependencies ([#1503](https://github.com/EnterpriseDB/klio/issues/1503)) ([a0af152](https://github.com/cloudnative-pg/klio/commit/a0af1528b2e69f7b3e115e7f7d1ef791168100c2))
* **deps:** Update all non-major go dependencies ([#1535](https://github.com/EnterpriseDB/klio/issues/1535)) ([146bb94](https://github.com/cloudnative-pg/klio/commit/146bb944fb1ac2ecafe832c2619064bb0f6a348d))
* **deps:** Update all non-major go dependencies and align otel semconv ([#1566](https://github.com/EnterpriseDB/klio/issues/1566)) ([0eca6a9](https://github.com/cloudnative-pg/klio/commit/0eca6a9edaa1e15eb74e00d56b56997553712ebc))
* **deps:** Update kubernetes monorepo to v0.36.1 ([#1485](https://github.com/EnterpriseDB/klio/issues/1485)) ([c09245b](https://github.com/cloudnative-pg/klio/commit/c09245b178c79c4b00b858d372962a9ec59bec69))
* **deps:** Update module github.com/cloudnative-pg/api to v1.29.1 ([#1481](https://github.com/EnterpriseDB/klio/issues/1481)) ([ebf6773](https://github.com/cloudnative-pg/klio/commit/ebf6773188a3c5dcbd9b6604ed151ad67e70af63))
* **deps:** Update module github.com/cloudnative-pg/machinery to v0.5.0 ([#1559](https://github.com/EnterpriseDB/klio/issues/1559)) ([c857b53](https://github.com/cloudnative-pg/klio/commit/c857b53cdbbed9f679de27ee3d6e04c8faddf491))
* **deps:** Update module github.com/nats-io/nats-server/v2 to v2.14.1 ([#1524](https://github.com/EnterpriseDB/klio/issues/1524)) ([a83f0e6](https://github.com/cloudnative-pg/klio/commit/a83f0e642ed374d3ae2d29b96e2c95e549c8cdc7))
* **deps:** Update module golang.org/x/crypto to v0.52.0 ([#1530](https://github.com/EnterpriseDB/klio/issues/1530)) ([c6a22ba](https://github.com/cloudnative-pg/klio/commit/c6a22ba61d4cd30043b74db131607292c3a2cf54))
* **deps:** Update module google.golang.org/grpc to v1.81.1 ([#1495](https://github.com/EnterpriseDB/klio/issues/1495)) ([a6c4aab](https://github.com/cloudnative-pg/klio/commit/a6c4aabc60124fc5d22db7af3c285547bb28e9a7))
* **deps:** Update module sigs.k8s.io/controller-runtime to v0.24.1 ([#1482](https://github.com/EnterpriseDB/klio/issues/1482)) ([6eaaf97](https://github.com/cloudnative-pg/klio/commit/6eaaf976e88a0e05a009979a8a5a503f2bf7c78c))
* **otel:** Guard against nil RootEntry in snapshot metrics ([#1571](https://github.com/EnterpriseDB/klio/issues/1571)) ([c18a799](https://github.com/cloudnative-pg/klio/commit/c18a799cdfb493098733ac05fb93eeaa25b4136e))


### Performance Improvements

* Larger buffer and direct writes for tier 2 archival ([#1548](https://github.com/EnterpriseDB/klio/issues/1548)) ([9cf9629](https://github.com/cloudnative-pg/klio/commit/9cf9629ab922313e98cf602f23f93746f645c905))
* Larger gRPC windows for WAL streaming ([bfe867d](https://github.com/cloudnative-pg/klio/commit/bfe867d34b55d263827e34e1eaf35ee396cdd313))


### Code Refactoring

* **otel:** Adopt component-based metric taxonomy ([#1513](https://github.com/EnterpriseDB/klio/issues/1513)) ([e1bc054](https://github.com/cloudnative-pg/klio/commit/e1bc054471303d86dbf0e3c1c13099b99142f0e4))

## [0.0.15](https://github.com/EnterpriseDB/klio/compare/v0.0.14...v0.0.15) (2026-05-08)


### Features

* Log progress of snapshot directory ([#1314](https://github.com/EnterpriseDB/klio/issues/1314)) ([eeeb58a](https://github.com/cloudnative-pg/klio/commit/eeeb58a468417e2c7d9b7c21b7a4456ae36ac173))
* **olm:** Add CNPG plugin service and gRPC TLS configuration ([#1325](https://github.com/EnterpriseDB/klio/issues/1325)) ([feb7773](https://github.com/cloudnative-pg/klio/commit/feb7773493660d9cdfd3ecb2654f26834d4dc6f1))
* **operator:** Add OpenShift SCC compatibility ([#1308](https://github.com/EnterpriseDB/klio/issues/1308)) ([dc2abc2](https://github.com/cloudnative-pg/klio/commit/dc2abc2132654f089c19879205488bb6cfc53cd8))
* **operator:** Derive CNPG group/version from cluster TypeMeta at runtime ([#1414](https://github.com/EnterpriseDB/klio/issues/1414)) ([6e50af2](https://github.com/cloudnative-pg/klio/commit/6e50af2ce854f2fbe8d14d9b0407de3788bba673))
* **otel:** Add latest written time metrics ([25e02a2](https://github.com/cloudnative-pg/klio/commit/25e02a21a016e103aac42ac3cf030662250cbdd6))
* **otel:** Add nats messages and bytes metrics ([#1388](https://github.com/EnterpriseDB/klio/issues/1388)) ([1bae678](https://github.com/cloudnative-pg/klio/commit/1bae678b268c531ccccc09915cdf237240d48a1e))
* **plugin:** Use Pre hook to requeue when PluginConfiguration is missing ([#1152](https://github.com/EnterpriseDB/klio/issues/1152)) ([2276ede](https://github.com/cloudnative-pg/klio/commit/2276edea6ee973e305715f2b903ed45064280b9e))


### Bug Fixes

* **deps:** Update all non-major go dependencies ([#1223](https://github.com/EnterpriseDB/klio/issues/1223)) ([90afc70](https://github.com/cloudnative-pg/klio/commit/90afc70d9963bb3986811b0c84774024dccfd8f1))
* **deps:** Update all non-major go dependencies ([#1259](https://github.com/EnterpriseDB/klio/issues/1259)) ([c9ddae0](https://github.com/cloudnative-pg/klio/commit/c9ddae040a3c79a6a6471ee26d1e98b40826aba4))
* **deps:** Update all non-major go dependencies ([#1275](https://github.com/EnterpriseDB/klio/issues/1275)) ([c9d5b5c](https://github.com/cloudnative-pg/klio/commit/c9d5b5c34850a15127abae2c8147d14a7e482f89))
* **deps:** Update all non-major go dependencies ([#1287](https://github.com/EnterpriseDB/klio/issues/1287)) ([3b8d6c1](https://github.com/cloudnative-pg/klio/commit/3b8d6c16515809c2323f2824831aabc8b9acb847))
* **deps:** Update all non-major go dependencies ([#1372](https://github.com/EnterpriseDB/klio/issues/1372)) ([4b70c9c](https://github.com/cloudnative-pg/klio/commit/4b70c9ccd271462641bfecee8efa14124394e1d2))
* **deps:** Update all non-major go dependencies ([#1376](https://github.com/EnterpriseDB/klio/issues/1376)) ([395ac8b](https://github.com/cloudnative-pg/klio/commit/395ac8b3e16ab5c89df78dd053e2d4b795663b75))
* **deps:** Update all non-major go dependencies ([#1384](https://github.com/EnterpriseDB/klio/issues/1384)) ([0600873](https://github.com/cloudnative-pg/klio/commit/0600873c455481727d490fdaacd825efc336f0dd))
* **deps:** Update all non-major go dependencies ([#1449](https://github.com/EnterpriseDB/klio/issues/1449)) ([051515d](https://github.com/cloudnative-pg/klio/commit/051515d9fdb1fdcfc2b3e642cdc05c905f8611cf))
* **deps:** Update documentation dependencies to v3.10.0 ([#1231](https://github.com/EnterpriseDB/klio/issues/1231)) ([2029d0e](https://github.com/cloudnative-pg/klio/commit/2029d0e6d7c6cd811aaf8e8bed2c237ddc7a49b7))
* **deps:** Update documentation dependencies to v3.10.1 ([#1402](https://github.com/EnterpriseDB/klio/issues/1402)) ([6f5c477](https://github.com/cloudnative-pg/klio/commit/6f5c477b489090038a611517025b49e819e67c9f))
* **deps:** Update kubernetes monorepo to v0.35.4 ([#1273](https://github.com/EnterpriseDB/klio/issues/1273)) ([ec5ec35](https://github.com/cloudnative-pg/klio/commit/ec5ec35367417896fa7c50738d3b3529e33d5b0a))
* **deps:** Update module github.com/aws/aws-sdk-go-v2/service/s3 to v1.100.0 ([#1321](https://github.com/EnterpriseDB/klio/issues/1321)) ([3574f35](https://github.com/cloudnative-pg/klio/commit/3574f3552202512f2898bd841d616f6c1ee29b91))
* **deps:** Update module github.com/aws/aws-sdk-go-v2/service/s3 to v1.99.0 ([#1208](https://github.com/EnterpriseDB/klio/issues/1208)) ([b22fe07](https://github.com/cloudnative-pg/klio/commit/b22fe07a50cce21429fa9a7cfe0511f18ad7d008))
* **deps:** Update module github.com/cert-manager/cert-manager to v1.20.2 ([#1237](https://github.com/EnterpriseDB/klio/issues/1237)) ([044d4ea](https://github.com/cloudnative-pg/klio/commit/044d4ea6b66569eeacaaac3b9c75be0c23392ea1))
* **deps:** Update module github.com/cloudnative-pg/machinery to v0.4.0 ([#1254](https://github.com/EnterpriseDB/klio/issues/1254)) ([9559fb9](https://github.com/cloudnative-pg/klio/commit/9559fb9056f62053938230c658ae5688472d4df4))
* **deps:** Update module github.com/jackc/pgx/v5 to v5.9.2 ([#1293](https://github.com/EnterpriseDB/klio/issues/1293)) ([a57f59e](https://github.com/cloudnative-pg/klio/commit/a57f59e2d1283190254ec6312da92460971542b7))
* **deps:** Update module github.com/minio/sio to v0.5.1 ([#1378](https://github.com/EnterpriseDB/klio/issues/1378)) ([8bc1752](https://github.com/cloudnative-pg/klio/commit/8bc17527433cd2f5d5c267e450b010495236a4f4))
* **deps:** Update module github.com/nats-io/nats-server/v2 to v2.12.8 ([#1363](https://github.com/EnterpriseDB/klio/issues/1363)) ([d3e7334](https://github.com/cloudnative-pg/klio/commit/d3e73340777f3c160ddab92748d28072e565b0e2))
* **deps:** Update module github.com/onsi/ginkgo/v2 to v2.28.2 ([#1355](https://github.com/EnterpriseDB/klio/issues/1355)) ([0458643](https://github.com/cloudnative-pg/klio/commit/0458643495b153cf38e8ebc1ca62ddd8ec49f45d))
* **deps:** Update module google.golang.org/grpc to v1.81.0 ([#1406](https://github.com/EnterpriseDB/klio/issues/1406)) ([c698faa](https://github.com/cloudnative-pg/klio/commit/c698faa5d52b61c198d5e70b7ff9de41d1d793b4))
* **deps:** Update module sigs.k8s.io/controller-runtime to v0.24.0 ([#1389](https://github.com/EnterpriseDB/klio/issues/1389)) ([14d9af3](https://github.com/cloudnative-pg/klio/commit/14d9af3784b585249cec2d5a2b6b74571ea454a5))
* **deps:** Update module sigs.k8s.io/e2e-framework to v0.7.0 ([#1307](https://github.com/EnterpriseDB/klio/issues/1307)) ([807b30c](https://github.com/cloudnative-pg/klio/commit/807b30c7d3822eb9eb444470271e4c2cca53d220))
* Determine --wait-for-wals based on instance role ([94047f8](https://github.com/cloudnative-pg/klio/commit/94047f8a328e85f993fedea55304e2703a486283))
* Fall back to replica source PluginConfiguration for sidecar ([#1381](https://github.com/EnterpriseDB/klio/issues/1381)) ([80fad5e](https://github.com/cloudnative-pg/klio/commit/80fad5e8edce3742c1daa2ac64025342dda34974))
* **operator:** Add RBAC for pluginconfigurations/finalizers ([#1334](https://github.com/EnterpriseDB/klio/issues/1334)) ([1d8ea0f](https://github.com/cloudnative-pg/klio/commit/1d8ea0f21ee0cb0b61514a02c505605be18fce89))
* **operator:** Handle nil FileReference in buildFileSourceVolMount ([#1214](https://github.com/EnterpriseDB/klio/issues/1214)) ([2771b17](https://github.com/cloudnative-pg/klio/commit/2771b1701697638cba2afe976e9c86f8fc5b841e))
* **otel:** Only initialize OpenTelemetry for long-running commands ([#1415](https://github.com/EnterpriseDB/klio/issues/1415)) ([d40160d](https://github.com/cloudnative-pg/klio/commit/d40160d6df83e93f57357688da6d054d385aa065))
* **otel:** Register tier2 written wals ([9e0c054](https://github.com/cloudnative-pg/klio/commit/9e0c054a2fcc22165524ca1206942e070da44889))

## [0.0.14](https://github.com/EnterpriseDB/klio/compare/v0.0.13...v0.0.14) (2026-04-07)


### ⚠ BREAKING CHANGES

* **core,operator:** The encryptionKey field has been removed from Server CRD. Users must migrate to encryptionKeyFile and identityFile with Age-encrypted keys. See documentation for migration steps.

### Features

* **backup:** Add snapshot pinning support for tier2 backups ([#1078](https://github.com/EnterpriseDB/klio/issues/1078)) ([ff2abab](https://github.com/cloudnative-pg/klio/commit/ff2abab52142eaaed20168e809ed73c860b5e173))
* **backup:** Add kopia snapshot verification after backup ([#1068](https://github.com/EnterpriseDB/klio/issues/1068)) ([a2dc6f5](https://github.com/cloudnative-pg/klio/commit/a2dc6f5c732fcf6e562800aada4ee76ad6090b65))
* **core,operator:** Use Age encryption for backup encryption keys ([#1087](https://github.com/EnterpriseDB/klio/issues/1087)) ([06604a9](https://github.com/cloudnative-pg/klio/commit/06604a90b441eb3981dd01fa52dd8c46fb9b9546))
* **core:** Add backup lifecycle metrics to CNPG plugin ([#1042](https://github.com/EnterpriseDB/klio/issues/1042)) ([f2841d2](https://github.com/cloudnative-pg/klio/commit/f2841d20d1d076106824c80de6d8173abdb60113))
* **operator:** Support online PVC expansion for Server resources ([#1030](https://github.com/EnterpriseDB/klio/issues/1030)) ([f3a6063](https://github.com/cloudnative-pg/klio/commit/f3a60631ba7cfa0cad892a6ca00ca37ec7cb3511))


### Bug Fixes

* Add LICENSE file to licenses root for preflight check ([#1138](https://github.com/EnterpriseDB/klio/issues/1138)) ([791bb9e](https://github.com/cloudnative-pg/klio/commit/791bb9e68b4fe922ddf7af0ef96efb344019779a))
* **core:** Lower minimum TLS version to 1.2 ([#1175](https://github.com/EnterpriseDB/klio/issues/1175)) ([725e7ac](https://github.com/cloudnative-pg/klio/commit/725e7ac02ef6d5f205e5cc16a34656ce0e260dcc))
* **deps:** Update all non-major go dependencies ([#1048](https://github.com/EnterpriseDB/klio/issues/1048)) ([12da898](https://github.com/cloudnative-pg/klio/commit/12da898bbb7a3a3dd2f53f560e6e9709c0384f61))
* **deps:** Update all non-major go dependencies ([#1056](https://github.com/EnterpriseDB/klio/issues/1056)) ([ac3c739](https://github.com/cloudnative-pg/klio/commit/ac3c7391bf3ca9c4f20308b81f7e1fe74571c8f9))
* **deps:** Update all non-major go dependencies ([#1147](https://github.com/EnterpriseDB/klio/issues/1147)) ([06630cd](https://github.com/cloudnative-pg/klio/commit/06630cd404e5daa5736590623350528c32ff8fad))
* **deps:** Update all non-major go dependencies ([#1158](https://github.com/EnterpriseDB/klio/issues/1158)) ([f97a988](https://github.com/cloudnative-pg/klio/commit/f97a9888b4938fd609e45258a916f4fa99c5f064))
* **deps:** Update all non-major go dependencies ([#1165](https://github.com/EnterpriseDB/klio/issues/1165)) ([999c4ee](https://github.com/cloudnative-pg/klio/commit/999c4ee2d081a926ee4f0dc0058efbf9b8bce695))
* **deps:** Update all non-major go dependencies ([#1174](https://github.com/EnterpriseDB/klio/issues/1174)) ([8332088](https://github.com/cloudnative-pg/klio/commit/8332088b1efb39aa5dbbd2f83624f12942e204fd))
* **deps:** Update all non-major go dependencies ([#1201](https://github.com/EnterpriseDB/klio/issues/1201)) ([fc8350e](https://github.com/cloudnative-pg/klio/commit/fc8350e808d156453b3fab969be8676687b84cab))
* **deps:** Update all non-major go dependencies to v1.43.0 ([#1178](https://github.com/EnterpriseDB/klio/issues/1178)) ([32ddabf](https://github.com/cloudnative-pg/klio/commit/32ddabf8d201634b5f5e03a4686dd51d09434920))
* **deps:** Update k8s.io/utils digest to 28399d8 ([#1097](https://github.com/EnterpriseDB/klio/issues/1097)) ([7b38dd6](https://github.com/cloudnative-pg/klio/commit/7b38dd62cdf62a891d5d7f23614d49a018e60f94))
* **deps:** Update Kopia fork to klio-20260407 ([#1198](https://github.com/EnterpriseDB/klio/issues/1198)) ([7a4f5c4](https://github.com/cloudnative-pg/klio/commit/7a4f5c403e2dcdd651de414274a3612fe8c380e6))
* **deps:** Update kubernetes monorepo to v0.35.3 ([#1096](https://github.com/EnterpriseDB/klio/issues/1096)) ([40d30f3](https://github.com/cloudnative-pg/klio/commit/40d30f3271481147c663acdc94c2aced0ca00978))
* **deps:** Update module github.com/aws/aws-sdk-go-v2/service/s3 to v1.97.2 ([#1123](https://github.com/EnterpriseDB/klio/issues/1123)) ([81620f7](https://github.com/cloudnative-pg/klio/commit/81620f77de6735c6ee3eed640c7adbaf92de7d5e))
* **deps:** Update module github.com/cert-manager/cert-manager to v1.20.0 ([#1036](https://github.com/EnterpriseDB/klio/issues/1036)) ([550e8c2](https://github.com/cloudnative-pg/klio/commit/550e8c277b0a31d879984099f1b520eb733fc40f))
* **deps:** Update module github.com/jackc/pgx/v5 to v5.9.0 ([#1109](https://github.com/EnterpriseDB/klio/issues/1109)) ([0fa0024](https://github.com/cloudnative-pg/klio/commit/0fa0024da63fae2cbd5b69dbabea20856ecfe616))
* **deps:** Update module github.com/jackc/pgx/v5 to v5.9.1 ([#1113](https://github.com/EnterpriseDB/klio/issues/1113)) ([fdbf675](https://github.com/cloudnative-pg/klio/commit/fdbf675ab0ecea988d5291f80a46e05a5ddf3b5c))
* **deps:** Update module github.com/klauspost/compress to v1.18.5 ([#1105](https://github.com/EnterpriseDB/klio/issues/1105)) ([661aed4](https://github.com/cloudnative-pg/klio/commit/661aed48c6f5df4c8796fa34e96dab76c27939cb))
* **deps:** Update module github.com/nats-io/nats-server/v2 to v2.12.5 ([#1024](https://github.com/EnterpriseDB/klio/issues/1024)) ([674f814](https://github.com/cloudnative-pg/klio/commit/674f814f5cb27afa1d131ff0063a78ed5e242169))
* **deps:** Update module github.com/nats-io/nats-server/v2 to v2.12.6 ([#1127](https://github.com/EnterpriseDB/klio/issues/1127)) ([3854f53](https://github.com/cloudnative-pg/klio/commit/3854f535d1b8d8518d7e2de63dbf0d238030369f))
* **deps:** Update module github.com/nats-io/nats.go to v1.50.0 ([#1132](https://github.com/EnterpriseDB/klio/issues/1132)) ([ba23127](https://github.com/cloudnative-pg/klio/commit/ba23127577fddc388edcf3b6859f58c941d2d4db))
* **deps:** Update module google.golang.org/grpc to v1.79.3 ([#1084](https://github.com/EnterpriseDB/klio/issues/1084)) ([ea73248](https://github.com/cloudnative-pg/klio/commit/ea73248b5d2c353cc44b628a3934f7f6b340c161))
* **operator:** Handle StatefulSet recreation when object never existed ([#1153](https://github.com/EnterpriseDB/klio/issues/1153)) ([ffd0b1d](https://github.com/cloudnative-pg/klio/commit/ffd0b1db20b93ebfd70272c6047bd6a61d0a19ba))
* **otel:** Align semconv version to otel v1.42.0 ([b00f667](https://github.com/cloudnative-pg/klio/commit/b00f667e27025a08d1c3dde2137f143e50ae920a))
* **otel:** Initialize OpenTelemetry after logging is configured ([bf90cbb](https://github.com/cloudnative-pg/klio/commit/bf90cbb91f4c7ecdda250fd4aff3c9118bbe2fac))

## [0.0.13](https://github.com/EnterpriseDB/klio/compare/v0.0.12...v0.0.13) (2026-03-09)


### ⚠ BREAKING CHANGES

* **operator:** clusterName is now a required field (minLength=1) in PluginConfiguration. Existing resources that omit it will fail validation.

### Features

* **admin:** Add delete-backup command for manual backup deletion ([#880](https://github.com/EnterpriseDB/klio/issues/880)) ([7c9a619](https://github.com/cloudnative-pg/klio/commit/7c9a6192d861511eeac5fbbda05200fbe3242833))
* **api:** Make CNPG API group and version configurable ([#922](https://github.com/EnterpriseDB/klio/issues/922)) ([0b2b381](https://github.com/cloudnative-pg/klio/commit/0b2b381e2eafb542dc3acfca62be05510f9a3ab4))
* **cnpgi:** Add WAL prefetching during recovery ([#889](https://github.com/EnterpriseDB/klio/issues/889)) ([5133164](https://github.com/cloudnative-pg/klio/commit/51331644b8f0706bc7900c775fa2970db8a9d84f))
* **cnpgi:** Reuse gRPC connections for WAL restore ([#888](https://github.com/EnterpriseDB/klio/issues/888)) ([0a5503f](https://github.com/cloudnative-pg/klio/commit/0a5503fb3b0d32e969dd6d76d8332df2678a80a0))
* **kopia,restore:** Enable progress output and split scan lines ([#923](https://github.com/EnterpriseDB/klio/issues/923)) ([6753649](https://github.com/cloudnative-pg/klio/commit/675364952d68cb2ea8f432053f87d1bbea7a1a37))
* **operator:** Add metadata field to Server pod template ([#920](https://github.com/EnterpriseDB/klio/issues/920)) ([2767bba](https://github.com/cloudnative-pg/klio/commit/2767bba460e81bc5d849504f3abc4ae9bda1bcc1))
* **operator:** Add unit tests to CI pipeline ([#976](https://github.com/EnterpriseDB/klio/issues/976)) ([bb4ac4f](https://github.com/cloudnative-pg/klio/commit/bb4ac4f75275e664f30ae566da44bf34f104efa7))
* **operator:** Auto-propagate PluginConfiguration changes to running pods ([#924](https://github.com/EnterpriseDB/klio/issues/924)) ([944c68e](https://github.com/cloudnative-pg/klio/commit/944c68e7489e7b937f6b9bace68998f6fba5d12f))
* Set progress update interval while migrating snapshots ([#964](https://github.com/EnterpriseDB/klio/issues/964)) ([a59efbf](https://github.com/cloudnative-pg/klio/commit/a59efbf0bc83056fcbbc4f6989720a2556d57a58))


### Bug Fixes

* Avoid showing usage information on every error ([#925](https://github.com/EnterpriseDB/klio/issues/925)) ([f0f0f8b](https://github.com/cloudnative-pg/klio/commit/f0f0f8b6b126d12fa4fa28fadd33544050f66da9))
* **core:** Disable Kopia cache for remote repository connections ([#966](https://github.com/EnterpriseDB/klio/issues/966)) ([bd4d15a](https://github.com/cloudnative-pg/klio/commit/bd4d15a9908e941c3e1608a2fa5704fe2eb37ad2))
* **core:** Disable Kopia update checks in all commands ([#965](https://github.com/EnterpriseDB/klio/issues/965)) ([bcdb29c](https://github.com/cloudnative-pg/klio/commit/bcdb29cb4ac62bd6cd16e9474c48db89efe6d78c))
* **core:** Inherit parent environment in Kopia commands ([#974](https://github.com/EnterpriseDB/klio/issues/974)) ([db22ac0](https://github.com/cloudnative-pg/klio/commit/db22ac0b40f887972655ee34b091e5841cfee487))
* **core:** Make NATS JetStream consumers durable ([#983](https://github.com/EnterpriseDB/klio/issues/983)) ([13dad5d](https://github.com/cloudnative-pg/klio/commit/13dad5d76daad8602509cba657cb8fe7b88a41e2))
* **core:** Restore environment variable bindings for custom CNPG group and version ([#863](https://github.com/EnterpriseDB/klio/issues/863)) ([7c74976](https://github.com/cloudnative-pg/klio/commit/7c7497694434e9a07780c740ad40857c64dc10c9))
* **deps:** Update all non-major go dependencies ([#875](https://github.com/EnterpriseDB/klio/issues/875)) ([64da957](https://github.com/cloudnative-pg/klio/commit/64da957befc63c5b842be2717a2e5194cab61231))
* **deps:** Update all non-major go dependencies ([#883](https://github.com/EnterpriseDB/klio/issues/883)) ([7a30e78](https://github.com/cloudnative-pg/klio/commit/7a30e7807befe3d4c0c07b6691c0f91c1d32be8c))
* **deps:** Update all non-major go dependencies ([#908](https://github.com/EnterpriseDB/klio/issues/908)) ([cd994da](https://github.com/cloudnative-pg/klio/commit/cd994daefe02cce74c019b028bd2b73320739b1f))
* **deps:** Update all non-major go dependencies ([#913](https://github.com/EnterpriseDB/klio/issues/913)) ([d85e780](https://github.com/cloudnative-pg/klio/commit/d85e7804bdbf7f79e7e29caf18c9531063615010))
* **deps:** Update all non-major go dependencies ([#962](https://github.com/EnterpriseDB/klio/issues/962)) ([a5e9c94](https://github.com/cloudnative-pg/klio/commit/a5e9c94b70ae7064090f0a1f7a3f2d3161bc8a1a))
* **deps:** Update all non-major go dependencies ([#971](https://github.com/EnterpriseDB/klio/issues/971)) ([98b23c2](https://github.com/cloudnative-pg/klio/commit/98b23c2c5e816b36120f258b89731d2da5891e1b))
* **deps:** Update all non-major go dependencies ([#999](https://github.com/EnterpriseDB/klio/issues/999)) ([58af956](https://github.com/cloudnative-pg/klio/commit/58af95646ae1e5c21ae3acb12ca68f2c32bcd276))
* **deps:** Update all non-major go dependencies to v1.41.0 ([#960](https://github.com/EnterpriseDB/klio/issues/960)) ([23584f9](https://github.com/cloudnative-pg/klio/commit/23584f958e239c3fe9d8b6517f86436e4937ac24))
* **deps:** Update dependency @easyops-cn/docusaurus-search-local to ^0.55.0 ([#862](https://github.com/EnterpriseDB/klio/issues/862)) ([156cc8a](https://github.com/cloudnative-pg/klio/commit/156cc8a5529a3f226589f6c019e55a5c20746b24))
* **deps:** Update kubernetes packages to v0.35.2 ([#940](https://github.com/EnterpriseDB/klio/issues/940)) ([b28a1fb](https://github.com/cloudnative-pg/klio/commit/b28a1fbb91794526972370f3cfa575c88a5de917))
* **deps:** Update module github.com/aws/aws-sdk-go-v2/service/s3 to v1.96.2 ([#934](https://github.com/EnterpriseDB/klio/issues/934)) ([7bfb588](https://github.com/cloudnative-pg/klio/commit/7bfb5889d205ed4fb287b9ece81fd5ea073ccb52))
* **deps:** Update module github.com/aws/aws-sdk-go-v2/service/s3 to v1.96.4 ([#989](https://github.com/EnterpriseDB/klio/issues/989)) ([fc0db9c](https://github.com/cloudnative-pg/klio/commit/fc0db9cfb729c3fe0eb015fa245d99a2ab90d890))
* **deps:** Update module golang.org/x/sync to v0.20.0 ([#1014](https://github.com/EnterpriseDB/klio/issues/1014)) ([a3d6c4b](https://github.com/cloudnative-pg/klio/commit/a3d6c4b6ca59c95b1fe2ed4b4db413cead25f095))
* **deps:** Update module google.golang.org/grpc to v1.79.1 ([#843](https://github.com/EnterpriseDB/klio/issues/843)) ([1ddb780](https://github.com/cloudnative-pg/klio/commit/1ddb780ad3aa6f3225f58b510f5873a97bd3045f))
* **deps:** Update module google.golang.org/grpc to v1.79.2 ([#995](https://github.com/EnterpriseDB/klio/issues/995)) ([c7853ac](https://github.com/cloudnative-pg/klio/commit/c7853ac7108392d6509e5bb8d24d957f1129c582))
* **deps:** Update module sigs.k8s.io/controller-runtime to v0.23.3 ([#986](https://github.com/EnterpriseDB/klio/issues/986)) ([875e946](https://github.com/cloudnative-pg/klio/commit/875e9467a942f9a6f193cfbc5af4abe1fd01d265))
* **kopia:** Retry scanner on error to prevent subprocess blocking ([#919](https://github.com/EnterpriseDB/klio/issues/919)) ([1518ece](https://github.com/cloudnative-pg/klio/commit/1518ece0c8a446e4a1993dc940704952c579d34f))
* **logger:** Avoid to log empty lines ([#957](https://github.com/EnterpriseDB/klio/issues/957)) ([9b4b563](https://github.com/cloudnative-pg/klio/commit/9b4b563b8837905291c275311277c9083f8965f8))
* **operator:** Make Tier1 and Tier2 fields optional pointers in PluginConfiguration ([#921](https://github.com/EnterpriseDB/klio/issues/921)) ([d5b25d9](https://github.com/cloudnative-pg/klio/commit/d5b25d920573a685ff70bb38d660b2f47ec77b71))
* **operator:** Use API rejection to recreate StatefulSet on immutable field changes ([#967](https://github.com/EnterpriseDB/klio/issues/967)) ([ae659bf](https://github.com/cloudnative-pg/klio/commit/ae659bfa4088d86928e59ddba1cca2669f9f2857))
* **server:** Enforce read-only at repository connection level ([#836](https://github.com/EnterpriseDB/klio/issues/836)) ([5c7a056](https://github.com/cloudnative-pg/klio/commit/5c7a056179591f7849a78ef6e5ac9a334f360c0e))
* **server:** Use separate RO and RW tier2 Kopia configs ([#870](https://github.com/EnterpriseDB/klio/issues/870)) ([745a9f6](https://github.com/cloudnative-pg/klio/commit/745a9f6220c1cf38c6eeec69542f5fcdfa902d6a))

## [0.0.12](https://github.com/EnterpriseDB/klio/compare/v0.0.11...v0.0.12) (2026-02-12)


### ⚠ BREAKING CHANGES

* **operator:** replace boolean ReadOnly with string Mode ([#790](https://github.com/EnterpriseDB/klio/issues/790))
* **operator:** RecoverySource CRD has been removed. Migrate to using Server with readOnly: true for recovery-only scenarios.

### Features

* **core:** Add admin server with Unix socket security ([#709](https://github.com/EnterpriseDB/klio/issues/709)) ([668c025](https://github.com/cloudnative-pg/klio/commit/668c02582ad7961eb1dc3b788453be65e336f639))
* **core:** Add Kopia server control authentication and cache refresh ([#643](https://github.com/EnterpriseDB/klio/issues/643)) ([32d14f2](https://github.com/cloudnative-pg/klio/commit/32d14f20f99c480e7446c73b4a1fa9185fec7c72))
* **core:** Add queue-status admin command ([#768](https://github.com/EnterpriseDB/klio/issues/768)) ([38bbaf9](https://github.com/cloudnative-pg/klio/commit/38bbaf937ec1d7b44cd278d5bff47ff111a096df))
* **core:** Add structured logging for Kopia command output ([#644](https://github.com/EnterpriseDB/klio/issues/644)) ([dc6e96a](https://github.com/cloudnative-pg/klio/commit/dc6e96a6c102e98692f10a572e0d8f037b39cf42))
* **core:** Validate cluster name matches certificate CN ([#780](https://github.com/EnterpriseDB/klio/issues/780)) ([d880147](https://github.com/cloudnative-pg/klio/commit/d88014769e9f5f637ab0ff7975813483682f6be5))
* **docs:** Add automated CLI documentation generation ([#708](https://github.com/EnterpriseDB/klio/issues/708)) ([9baec3a](https://github.com/cloudnative-pg/klio/commit/9baec3a8ec0f31f7d4ec12291b6958c43c241f42))
* **e2e:** Add tier2 PITR recovery test ([#781](https://github.com/EnterpriseDB/klio/issues/781)) ([8520c22](https://github.com/cloudnative-pg/klio/commit/8520c22c0c2e6c85fa71faed8c802e9018838e9a))
* Ensure retention policies are applied on WALs ([#731](https://github.com/EnterpriseDB/klio/issues/731)) ([a9a9468](https://github.com/cloudnative-pg/klio/commit/a9a946824c9afc6d01db7a73e27efbee7a982388))
* **kopia:** Add option to disable Kopia UI ([#673](https://github.com/EnterpriseDB/klio/issues/673)) ([9f8ed01](https://github.com/cloudnative-pg/klio/commit/9f8ed013d4b9eaaf5a4597d2df1784acbb5c0cc0))
* **operator:** Replace boolean ReadOnly with string Mode ([#790](https://github.com/EnterpriseDB/klio/issues/790)) ([500bec1](https://github.com/cloudnative-pg/klio/commit/500bec118f65ab31ab10f78e43aaa413c390d10a))


### Bug Fixes

* **cnpgi:** Respect debug flag in WAL capability ([#698](https://github.com/EnterpriseDB/klio/issues/698)) ([99a070c](https://github.com/cloudnative-pg/klio/commit/99a070ca80882feab99bb8fb6c18f0d8e599b69e))
* **core:** Handle graceful shutdown for restore job sidecar ([#676](https://github.com/EnterpriseDB/klio/issues/676)) ([252cbbb](https://github.com/cloudnative-pg/klio/commit/252cbbbc4c5d57de0b1c71a3e1864ccabe5ad420))
* **core:** Include CA certificates in container image ([#766](https://github.com/EnterpriseDB/klio/issues/766)) ([096a0fd](https://github.com/cloudnative-pg/klio/commit/096a0fdeabc7973cf78a703c243945bb4f9dc1da))
* **core:** Make S3 endpoint optional in validation ([#767](https://github.com/EnterpriseDB/klio/issues/767)) ([a540b7d](https://github.com/cloudnative-pg/klio/commit/a540b7de815114e78ea2cf21058484f1f73012d8))
* **core:** Restore ServerControlCredential struct for Kopia server authentication ([#685](https://github.com/EnterpriseDB/klio/issues/685)) ([edbac7d](https://github.com/cloudnative-pg/klio/commit/edbac7d24ead8863ff81df257d2bbca3ef29d413))
* **core:** Tier1 WAL retention no longer considers tier2 backups ([#752](https://github.com/EnterpriseDB/klio/issues/752)) ([967b6be](https://github.com/cloudnative-pg/klio/commit/967b6bead82522a35345c79b8d52318a8fd4b9c9))
* **core:** Wal server graceful termination ([#706](https://github.com/EnterpriseDB/klio/issues/706)) ([aa1393e](https://github.com/cloudnative-pg/klio/commit/aa1393ea3d97f465638e66ca423951cb81521f5c))
* **deps:** Update all non-major go dependencies ([#751](https://github.com/EnterpriseDB/klio/issues/751)) ([455198f](https://github.com/cloudnative-pg/klio/commit/455198f67b8112fcd9b121522b5695fccfe4d9eb))
* **deps:** Update all non-major go dependencies ([#772](https://github.com/EnterpriseDB/klio/issues/772)) ([caec5bf](https://github.com/cloudnative-pg/klio/commit/caec5bfa85af1e15065226b24fc865d56e60d6af))
* **deps:** Update all non-major go dependencies ([#813](https://github.com/EnterpriseDB/klio/issues/813)) ([8c0f17b](https://github.com/cloudnative-pg/klio/commit/8c0f17b03be2271e48bed1b1c8e45387d81e774a))
* **deps:** Update dependency @easyops-cn/docusaurus-search-local to ^0.53.0 ([#814](https://github.com/EnterpriseDB/klio/issues/814)) ([e9e8808](https://github.com/cloudnative-pg/klio/commit/e9e88080fc1baa1c46528ac59a31aa6ba6bd7e43))
* **deps:** Update dependency @easyops-cn/docusaurus-search-local to ^0.54.0 ([#835](https://github.com/EnterpriseDB/klio/issues/835)) ([3bd3c31](https://github.com/cloudnative-pg/klio/commit/3bd3c31c2ba586ebef2fc6e26d9c8ad608467adf))
* **deps:** Update k8s.io/utils digest to b8788ab ([#826](https://github.com/EnterpriseDB/klio/issues/826)) ([78cd7ac](https://github.com/cloudnative-pg/klio/commit/78cd7ac2fa77f6207b066bff5c72a16527bf00ba))
* **deps:** Update kubernetes packages to v0.35.1 ([#831](https://github.com/EnterpriseDB/klio/issues/831)) ([f97f318](https://github.com/cloudnative-pg/klio/commit/f97f3185999eda30be15e2e55da72128d5971a9a))
* **deps:** Update module github.com/aws/aws-sdk-go-v2/service/s3 to v1.96.0 ([#788](https://github.com/EnterpriseDB/klio/issues/788)) ([7bee981](https://github.com/cloudnative-pg/klio/commit/7bee981378181fb70b84715d4954722113ce157c))
* **deps:** Update module github.com/cert-manager/cert-manager to v1.19.3 [security] ([#774](https://github.com/EnterpriseDB/klio/issues/774)) ([6fca766](https://github.com/cloudnative-pg/klio/commit/6fca7664312c651c2ccb59b65fa38ea992e3da23))
* **deps:** Update module github.com/cloudnative-pg/cloudnative-pg to v1.28.1 ([#794](https://github.com/EnterpriseDB/klio/issues/794)) ([e4f87f6](https://github.com/cloudnative-pg/klio/commit/e4f87f691a8371c2bdcd7c83bbe73432d002d810))
* **deps:** Update module github.com/klauspost/compress to v1.18.3 ([#656](https://github.com/EnterpriseDB/klio/issues/656)) ([ca6e3cc](https://github.com/cloudnative-pg/klio/commit/ca6e3ccca2e0ea80618343217db55a71b3cedb20))
* **deps:** Update module github.com/nats-io/nats-server/v2 to v2.12.4 ([#735](https://github.com/EnterpriseDB/klio/issues/735)) ([424d430](https://github.com/cloudnative-pg/klio/commit/424d4302b4d52d3702648a4237a9b605bd2b5fa6))
* **deps:** Update module sigs.k8s.io/controller-runtime to v0.23.0 ([#683](https://github.com/EnterpriseDB/klio/issues/683)) ([800b897](https://github.com/cloudnative-pg/klio/commit/800b89793fe70408f320f2c3dbcec2e47469dedc))
* **deps:** Update module sigs.k8s.io/controller-runtime to v0.23.1 ([#728](https://github.com/EnterpriseDB/klio/issues/728)) ([3d9c62a](https://github.com/cloudnative-pg/klio/commit/3d9c62a7dcc808c98ae582285638be0cab0f60ad))
* Enable fsync for JetStream to ensure durability ([#653](https://github.com/EnterpriseDB/klio/issues/653)) ([e6132f0](https://github.com/cloudnative-pg/klio/commit/e6132f0f20b5b3469d887e12f59552c17239e7e5))
* **operator:** Only set optional S3 env vars when non-empty ([#764](https://github.com/EnterpriseDB/klio/issues/764)) ([8c6d47b](https://github.com/cloudnative-pg/klio/commit/8c6d47b8cb06d1b26271835709f5fc4cb985234b))
* **security:** Avoid PATH dependency for klio command invocations ([caa6b4a](https://github.com/cloudnative-pg/klio/commit/caa6b4afbb217358c07dca53c367073ed1318d61))


### Code Refactoring

* **operator:** Consolidate RecoverySource into read-only Server support ([#696](https://github.com/EnterpriseDB/klio/issues/696)) ([034051b](https://github.com/cloudnative-pg/klio/commit/034051bc6b14f4225b43514b7124da9b565bf79c))

## [0.0.11](https://github.com/EnterpriseDB/klio/compare/v0.0.10...v0.0.11) (2026-01-16)


### ⚠ BREAKING CHANGES

* **api:** tier1 configuration has been moved to the spec.tier1 stanza; encryption_password has been renamed to encryption_key
* spec.baseConfiguration, spec.resources and spec.queueConfiguration.resources have been removed from the API.
* spec.baseConfiguration.adminUser has been removed from the Server API.

### Features

* **core,operator:** Implement tier2 retention policy support ([#615](https://github.com/EnterpriseDB/klio/issues/615)) ([e5fca21](https://github.com/cloudnative-pg/klio/commit/e5fca2145de82448b2d4f4d5c0060d4d1523b981))
* Introduce RecoverySource CRD ([3884720](https://github.com/cloudnative-pg/klio/commit/388472085ac309ccb59d358607f87ca096590959))
* Remove admin user and password Kopia settings ([#543](https://github.com/EnterpriseDB/klio/issues/543)) ([b0588a5](https://github.com/cloudnative-pg/klio/commit/b0588a5099805fe5254ab67aada4da015f9c7f0b))
* Remove container resources configuration from API ([#545](https://github.com/EnterpriseDB/klio/issues/545)) ([bf4d3b0](https://github.com/cloudnative-pg/klio/commit/bf4d3b0adee14d35c883bbac8a32835442037aed))


### Bug Fixes

* **deps:** Update all non-major go dependencies ([#538](https://github.com/EnterpriseDB/klio/issues/538)) ([eccbfe3](https://github.com/cloudnative-pg/klio/commit/eccbfe3d01f8570b438315b6ff122b3cd0b71d0b))
* **deps:** Update all non-major go dependencies ([#552](https://github.com/EnterpriseDB/klio/issues/552)) ([5fc1788](https://github.com/cloudnative-pg/klio/commit/5fc17884bae9af703a20891d741383c3b4a8fdbd))
* **deps:** Update all non-major go dependencies ([#599](https://github.com/EnterpriseDB/klio/issues/599)) ([7337dde](https://github.com/cloudnative-pg/klio/commit/7337dde915b43221e72b5fea30a537995f6b0d9b))
* **deps:** Update k8s.io/utils digest to 0fe9cd7 ([#590](https://github.com/EnterpriseDB/klio/issues/590)) ([8315738](https://github.com/cloudnative-pg/klio/commit/83157387dd61d80ade3f71cca60e2f89eb85fd52))
* **deps:** Update k8s.io/utils digest to 914a6e7 ([#600](https://github.com/EnterpriseDB/klio/issues/600)) ([811887f](https://github.com/cloudnative-pg/klio/commit/811887fdfcb3869681e3989c22d6464c7c738767))
* **deps:** Update module github.com/fclairamb/afero-s3 to v0.4.0 ([#610](https://github.com/EnterpriseDB/klio/issues/610)) ([585ece5](https://github.com/cloudnative-pg/klio/commit/585ece5664861ad4129e6c37a8609aa7ef85bfd1))
* **deps:** Update module github.com/onsi/ginkgo/v2 to v2.27.5 ([#622](https://github.com/EnterpriseDB/klio/issues/622)) ([6f60141](https://github.com/cloudnative-pg/klio/commit/6f6014173da0d6c3fb2a4f00d55463d1ac250831))
* **deps:** Update module golang.org/x/crypto to v0.47.0 ([#619](https://github.com/EnterpriseDB/klio/issues/619)) ([7992f6b](https://github.com/cloudnative-pg/klio/commit/7992f6ba42aeefe4ccc4686a903eed8cbb9f4f64))
* **integration:** Prevent Dagger from caching Klio helm deployments ([#635](https://github.com/EnterpriseDB/klio/issues/635)) ([bd7fbf4](https://github.com/cloudnative-pg/klio/commit/bd7fbf4631793d41fb3fa5ddfc0c59b6fb40cd68))
* **operator:** Add comprehensive error logging to lifecycle plugin ([#633](https://github.com/EnterpriseDB/klio/issues/633)) ([df4e445](https://github.com/cloudnative-pg/klio/commit/df4e445bb2b75282b72273bfef9ab5aaf5a85fa7))
* **operator:** Resolve server reconciliation issues ([#626](https://github.com/EnterpriseDB/klio/issues/626)) ([8f63cda](https://github.com/cloudnative-pg/klio/commit/8f63cdadf2480411a623ead690aca0e32ad47e01))


### Code Refactoring

* **api:** Add tier1 stanza and rename several spec fields ([#618](https://github.com/EnterpriseDB/klio/issues/618)) ([4e5f663](https://github.com/cloudnative-pg/klio/commit/4e5f663d98472be2fdcfdcaadd758f3ecfbbe2fc))

## [0.0.10](https://github.com/EnterpriseDB/klio/compare/v0.0.9...v0.0.10) (2025-12-24)


### ⚠ BREAKING CHANGES

* Removed spec.queueConfiguration.image from Server resource. The nats binary is now included in the Klio image.

### Features

* Implement tier2 storage support with backup and WAL synchronization ([#412](https://github.com/EnterpriseDB/klio/issues/412)) ([ada0e34](https://github.com/cloudnative-pg/klio/commit/ada0e34fc0396ca0e260622499d4e0b3ed9c7ea9))
* Remove nats image key from Klio server api ([#510](https://github.com/EnterpriseDB/klio/issues/510)) ([6a2de67](https://github.com/cloudnative-pg/klio/commit/6a2de6767090918428038dd377908db3af3e31bf))
* **tier2:** Restore ([#503](https://github.com/EnterpriseDB/klio/issues/503)) ([5aba140](https://github.com/cloudnative-pg/klio/commit/5aba1405eb390e82b127b185dc90ba12d89aa7bd))


### Bug Fixes

* Avoid using snapshot IDs in backup metadata ([#492](https://github.com/EnterpriseDB/klio/issues/492)) ([4e3d07d](https://github.com/cloudnative-pg/klio/commit/4e3d07d1a3ab902d14efa696efd06160295716b4))
* Clean up cache when initializing a new Kopia repository ([#483](https://github.com/EnterpriseDB/klio/issues/483)) ([cd55559](https://github.com/cloudnative-pg/klio/commit/cd5555941f074a85fb5b333a02330f69822bee79))
* **deps:** Update all non-major go dependencies ([#391](https://github.com/EnterpriseDB/klio/issues/391)) ([6560aa6](https://github.com/cloudnative-pg/klio/commit/6560aa6b58259c55f8bce78d3f6eb2c4874e661e))
* **deps:** Update all non-major go dependencies ([#524](https://github.com/EnterpriseDB/klio/issues/524)) ([ee22c03](https://github.com/cloudnative-pg/klio/commit/ee22c030176ba3341d9cc626c1bff4db753b43f2))
* **deps:** Update k8s.io/utils digest to 718f0e5 ([#532](https://github.com/EnterpriseDB/klio/issues/532)) ([8968b6a](https://github.com/cloudnative-pg/klio/commit/8968b6ac29338717050baf95769b05357a984cef))
* **deps:** Update k8s.io/utils digest to 9d40a56 ([#527](https://github.com/EnterpriseDB/klio/issues/527)) ([90e12b7](https://github.com/cloudnative-pg/klio/commit/90e12b74f3774628bc169a8fdbbb58ec59ba7b8c))
* **deps:** Update kubernetes packages to v0.34.3 ([#509](https://github.com/EnterpriseDB/klio/issues/509)) ([a32f810](https://github.com/cloudnative-pg/klio/commit/a32f8105461b089932b511f7509fe872c4429509))
* **deps:** Update kubernetes packages to v0.35.0 ([#535](https://github.com/EnterpriseDB/klio/issues/535)) ([4597160](https://github.com/cloudnative-pg/klio/commit/4597160e2e93d624accb1aa22267935628829f9a))
* **deps:** Update module github.com/cloudnative-pg/api to v1.28.0 ([#520](https://github.com/EnterpriseDB/klio/issues/520)) ([83eb2c9](https://github.com/cloudnative-pg/klio/commit/83eb2c9459d23b25d21623552fdf49f0d30f55d8))
* **deps:** Update module google.golang.org/grpc to v1.78.0 ([#536](https://github.com/EnterpriseDB/klio/issues/536)) ([8a9cc92](https://github.com/cloudnative-pg/klio/commit/8a9cc9213799d8a569d66172189da202c4a44aca))
* Make QueueConfiguration optional ([#493](https://github.com/EnterpriseDB/klio/issues/493)) ([f755a61](https://github.com/cloudnative-pg/klio/commit/f755a6138adc4a2f3e3f55dd4ccad8a705770884))
* Returning an error if closeMarkDone errors ([#482](https://github.com/EnterpriseDB/klio/issues/482)) ([06e3bf5](https://github.com/cloudnative-pg/klio/commit/06e3bf5489af73e80dd13c48c85b27afd50c7029))

## [0.0.9](https://github.com/EnterpriseDB/klio/compare/v0.0.8...v0.0.9) (2025-11-28)


### ⚠ BREAKING CHANGES

* hostname parameter dropped from Klio file configuration. No change for the operator.
* the `backupID` field in the `PluginConfiguration` object has been removed in favour of the `cluster.spec.bootstrap.recovery.recoveryTarget.backupID` field.

### Features

* Drop hostname from Klio configuration ([#445](https://github.com/EnterpriseDB/klio/issues/445)) ([bc3bd7b](https://github.com/cloudnative-pg/klio/commit/bc3bd7b1c39f742a3467574719ec2d730ae33f57))
* Remove support for backupID from plugin configuration ([#425](https://github.com/EnterpriseDB/klio/issues/425)) ([dff3ace](https://github.com/cloudnative-pg/klio/commit/dff3ace08918fe13b3ac6197ee1275437662c354))
* Send WAL uploaded events to NATS ([#403](https://github.com/EnterpriseDB/klio/issues/403)) ([7580871](https://github.com/cloudnative-pg/klio/commit/75808717925be0a47bbcac0792c1561b44393a0d))


### Bug Fixes

* **deps:** Update k8s.io/kube-openapi digest to 4e65d59 ([#470](https://github.com/EnterpriseDB/klio/issues/470)) ([203fb2e](https://github.com/cloudnative-pg/klio/commit/203fb2e94c51379ccd0d78953e21c0fcfe58b039))
* **deps:** Update k8s.io/kube-openapi digest to b6aabc6 ([#466](https://github.com/EnterpriseDB/klio/issues/466)) ([074e3a8](https://github.com/cloudnative-pg/klio/commit/074e3a81ea3a653fe2375bf571191005a97caae2))
* **deps:** Update kubernetes packages to v0.34.2 ([#455](https://github.com/EnterpriseDB/klio/issues/455)) ([8fb769f](https://github.com/cloudnative-pg/klio/commit/8fb769f38c5a4e15ed567e1197633eeede30961c))
* **deps:** Update module golang.org/x/crypto to v0.45.0 [security] ([#456](https://github.com/EnterpriseDB/klio/issues/456)) ([84dd26a](https://github.com/cloudnative-pg/klio/commit/84dd26abd3388077350b0bce30b4bd4e42d62024))

## [0.0.8](https://github.com/EnterpriseDB/klio/compare/v0.0.7...v0.0.8) (2025-11-07)


### Miscellaneous Chores

* Release 0.0.8 ([#428](https://github.com/EnterpriseDB/klio/issues/428)) ([e7c2214](https://github.com/EnterpriseDB/klio/commit/e7c2214bce6e751a47336a7fd2a812bd931df0be))

## [0.0.7](https://github.com/EnterpriseDB/klio/compare/v0.0.6...v0.0.7) (2025-11-07)


### ⚠ BREAKING CHANGES

* **recovery:** backupRef is removed from the PluginConfiguration API.
* **metrics,cnpgi:** Removed metrics-bind-address configuration options
* **auth:** previous configuration using htpasswd are no longer supported.

### Features

* Add TLS support in API service ([#339](https://github.com/EnterpriseDB/klio/issues/339)) ([902cd3f](https://github.com/cloudnative-pg/klio/commit/902cd3fcf74b1987b41cbc8c0ea182c39dceabbb))
* **auth:** Use mTLS and client-side certificate for authentication ([#366](https://github.com/EnterpriseDB/klio/issues/366)) ([deb57af](https://github.com/cloudnative-pg/klio/commit/deb57afc154d66f82e0265e999d38a322792e8ca))
* Change default images in Helm chart ([#382](https://github.com/EnterpriseDB/klio/issues/382)) ([3e64e71](https://github.com/cloudnative-pg/klio/commit/3e64e715c297ed0d3b5f9b0af974d6a5cc926423))
* **crd:** Add container customization to PluginConfiguration ([#388](https://github.com/EnterpriseDB/klio/issues/388)) ([603ada9](https://github.com/cloudnative-pg/klio/commit/603ada9826b3ea3674005f8c99c6123d3e5cc162))
* Enable ACLs ([#353](https://github.com/EnterpriseDB/klio/issues/353)) ([08dd43c](https://github.com/cloudnative-pg/klio/commit/08dd43c57c61ca3957544be8536e5d86ac4e4559))
* **metrics,cnpgi:** Route cnpgi controller-runtime metrics through OTEL ([#357](https://github.com/EnterpriseDB/klio/issues/357)) ([0cd1202](https://github.com/cloudnative-pg/klio/commit/0cd12023c3c9923955f97f226a73bf4c38481151))
* **recovery:** Drop backupRef support ([#409](https://github.com/EnterpriseDB/klio/issues/409)) ([d0c1fe5](https://github.com/cloudnative-pg/klio/commit/d0c1fe5e495311796de1e9289afee9addaf38ec8))
* Return 404 for missing WAL files ([#288](https://github.com/EnterpriseDB/klio/issues/288)) ([15ba9e4](https://github.com/cloudnative-pg/klio/commit/15ba9e4e19aff1cb78156736fc4e3f30493143c5))
* Set default k8s.* labels in containers ([#372](https://github.com/EnterpriseDB/klio/issues/372)) ([3ad634d](https://github.com/cloudnative-pg/klio/commit/3ad634db28bbc0d2af057b5a013128cd8cf19dd3))
* WAL server read-only mode ([#402](https://github.com/EnterpriseDB/klio/issues/402)) ([c7e9048](https://github.com/cloudnative-pg/klio/commit/c7e904820705fae6f18c7d51b067e5a5327d1dbf))


### Bug Fixes

* **deps:** Lock file maintenance documentation dependencies ([#361](https://github.com/EnterpriseDB/klio/issues/361)) ([3156410](https://github.com/cloudnative-pg/klio/commit/315641019832ff698bed47dba60e127cc5be9bfb))
* **deps:** Update module github.com/cloudnative-pg/cloudnative-pg to v1.27.1 ([#368](https://github.com/EnterpriseDB/klio/issues/368)) ([833aff1](https://github.com/cloudnative-pg/klio/commit/833aff1d4cf68c1ec347ef24c628530694e050e6))
* **deps:** Update module github.com/klauspost/compress to v1.18.1 ([#351](https://github.com/EnterpriseDB/klio/issues/351)) ([24a1def](https://github.com/cloudnative-pg/klio/commit/24a1defee40da97fdde83a58342e49a393c68c67))
* **deps:** Update module github.com/onsi/ginkgo/v2 to v2.27.1 ([#365](https://github.com/EnterpriseDB/klio/issues/365)) ([0962346](https://github.com/cloudnative-pg/klio/commit/0962346abb2dd7a53da17f8da22c09b8626a91ac))
* **deps:** Update module github.com/onsi/ginkgo/v2 to v2.27.2 ([#378](https://github.com/EnterpriseDB/klio/issues/378)) ([1ebbaeb](https://github.com/cloudnative-pg/klio/commit/1ebbaeba1b31030228809f36153a755d68e9c395))
* **deps:** Update module sigs.k8s.io/controller-runtime to v0.22.3 ([#358](https://github.com/EnterpriseDB/klio/issues/358)) ([57e545a](https://github.com/cloudnative-pg/klio/commit/57e545a827e5c31d8ec2860313ccaa4cbdae4974))
* **deps:** Update module sigs.k8s.io/controller-runtime to v0.22.4 ([#399](https://github.com/EnterpriseDB/klio/issues/399)) ([ad192b4](https://github.com/cloudnative-pg/klio/commit/ad192b4b8dfc971e530439fe8f9f50410cf175d9))
* Names and namespaces in replica clusters samples ([#385](https://github.com/EnterpriseDB/klio/issues/385)) ([214e6c6](https://github.com/cloudnative-pg/klio/commit/214e6c6f4c6a2a00ef792c0d5c3fd0f0f71f5e9a))

## [0.0.6](https://github.com/EnterpriseDB/klio/compare/v0.0.5...v0.0.6) (2025-10-20)


### ⚠ BREAKING CHANGES

* previous cluster configurations will not work unless a PluginConfiguration is defined and referred in the Cluster resource.

### Features

* Add CRD for plugin configuration parameters ([#327](https://github.com/EnterpriseDB/klio/issues/327)) ([a765541](https://github.com/cloudnative-pg/klio/commit/a765541726116ad0c4024e2c2673111089de339f))
* Clarify server initialize for existing directories ([#287](https://github.com/EnterpriseDB/klio/issues/287)) ([74ebc81](https://github.com/cloudnative-pg/klio/commit/74ebc8116498bbbec718ddd12b45a8d1b4748383))
* Improve tracing ([#289](https://github.com/EnterpriseDB/klio/issues/289)) ([773397a](https://github.com/cloudnative-pg/klio/commit/773397a4f7ede235619dd897c00ea9b1e88bb947))
* Make metrics addresses configurable ([#294](https://github.com/EnterpriseDB/klio/issues/294)) ([b907732](https://github.com/cloudnative-pg/klio/commit/b9077325068e1e0b488d61c488070d75c99816b2))
* Support custom sidecar image and custom api group ([#293](https://github.com/EnterpriseDB/klio/issues/293)) ([2622672](https://github.com/cloudnative-pg/klio/commit/262267236e726ebd97de9f1ba770a6d69061a4e0))


### Bug Fixes

* Always include cluster domain in the generated Certificates ([#335](https://github.com/EnterpriseDB/klio/issues/335)) ([a92d952](https://github.com/cloudnative-pg/klio/commit/a92d9523a6eabfd6dc5ddff1c4ced37354937e44))
* Deployment template had incorrect references ([#290](https://github.com/EnterpriseDB/klio/issues/290)) ([01b2703](https://github.com/cloudnative-pg/klio/commit/01b27036e82b4014ad9baae3cde7f4c01573817d))
* **deps:** Lock file maintenance documentation dependencies ([#303](https://github.com/EnterpriseDB/klio/issues/303)) ([1ccbafe](https://github.com/cloudnative-pg/klio/commit/1ccbafe015791b8a7990b77ec78f95d9e82efef8))
* **deps:** Update all non-major go dependencies ([#326](https://github.com/EnterpriseDB/klio/issues/326)) ([542b9b6](https://github.com/cloudnative-pg/klio/commit/542b9b6d202da486aff81fcf345373e9e58a6851))
* **deps:** Update all non-major go dependencies ([#345](https://github.com/EnterpriseDB/klio/issues/345)) ([b20eb21](https://github.com/cloudnative-pg/klio/commit/b20eb21f87e0cb9cc82d96e2df4b32f9eeca813a))
* **deps:** Update k8s.io/utils digest to bc988d5 ([#315](https://github.com/EnterpriseDB/klio/issues/315)) ([44ac2b5](https://github.com/cloudnative-pg/klio/commit/44ac2b52319ac010470a82acea65252f5c6d5587))
* **deps:** Update module github.com/onsi/ginkgo/v2 to v2.26.0 ([#321](https://github.com/EnterpriseDB/klio/issues/321)) ([c3b5f2c](https://github.com/cloudnative-pg/klio/commit/c3b5f2c1eeb99df423376d74bf7417cbeeed446a))
* **deps:** Update module google.golang.org/protobuf to v1.36.10 ([#314](https://github.com/EnterpriseDB/klio/issues/314)) ([3ecc6a4](https://github.com/cloudnative-pg/klio/commit/3ecc6a4b7eebbd8ec24f31879fc3e87f4d0f6e35))
* **deps:** Update module sigs.k8s.io/controller-runtime to v0.22.2 ([#325](https://github.com/EnterpriseDB/klio/issues/325)) ([4c08a73](https://github.com/cloudnative-pg/klio/commit/4c08a730d0dc769c08f51a23ae3807097d4adf33))
* Do not consider disabled plugins for tls generation ([#311](https://github.com/EnterpriseDB/klio/issues/311)) ([1d8e9da](https://github.com/cloudnative-pg/klio/commit/1d8e9da82be49aa88f8e5aef7ce79c9db7ef295b))
* Do not consider disabsled plugins for tls generation ([1d8e9da](https://github.com/cloudnative-pg/klio/commit/1d8e9da82be49aa88f8e5aef7ce79c9db7ef295b))

## [0.0.5](https://github.com/EnterpriseDB/klio/compare/v0.0.4...v0.0.5) (2025-09-26)


### Features

* Add opentelemetry metrics to core base and wal server ([#223](https://github.com/EnterpriseDB/klio/issues/223)) ([62acfd8](https://github.com/cloudnative-pg/klio/commit/62acfd81ff02782064a42b5081594d288541b008))
* Allow template override ([#258](https://github.com/EnterpriseDB/klio/issues/258)) ([978623e](https://github.com/cloudnative-pg/klio/commit/978623e8b62b7ddca650e7aacb371e590462c9f4))
* Allow using backupID to restore ([#254](https://github.com/EnterpriseDB/klio/issues/254)) ([ceeea70](https://github.com/cloudnative-pg/klio/commit/ceeea7092cf028fe31515cfce5246e323c98a290))
* API extension for backup catalog observability  ([#201](https://github.com/EnterpriseDB/klio/issues/201)) ([7264b8d](https://github.com/cloudnative-pg/klio/commit/7264b8d06f72537b8af55e61bf4e0f12a44d2cae))


### Bug Fixes

* Cleanup pg_wal directory after restore ([#266](https://github.com/EnterpriseDB/klio/issues/266)) ([7510048](https://github.com/cloudnative-pg/klio/commit/75100480d7482c53c77597f8bb21d6273252325a))
* Consider that the current object could have nil maps ([e4e74c3](https://github.com/cloudnative-pg/klio/commit/e4e74c31dfe902e6e71c068044f1e06da4a65632))
* Create service before sts ([5679297](https://github.com/cloudnative-pg/klio/commit/5679297d99e35d6982f1e0a667e7b134e007bd42))
* **deps:** Update all non-major go dependencies ([#247](https://github.com/EnterpriseDB/klio/issues/247)) ([1953052](https://github.com/cloudnative-pg/klio/commit/195305259fffcd10a3303c7106f756c6d85cce87))
* **deps:** Update all non-major go dependencies ([#271](https://github.com/EnterpriseDB/klio/issues/271)) ([dba8bc9](https://github.com/cloudnative-pg/klio/commit/dba8bc997dfe2abdd25b277251f9cf50dec6830a))
* **deps:** Update k8s.io/kube-openapi digest to 589584f ([#261](https://github.com/EnterpriseDB/klio/issues/261)) ([a01efbe](https://github.com/cloudnative-pg/klio/commit/a01efbeb839143149da4083993811753f2e3b83b))
* **deps:** Update kubernetes packages to v0.34.1 ([#253](https://github.com/EnterpriseDB/klio/issues/253)) ([fc90ace](https://github.com/cloudnative-pg/klio/commit/fc90ace0308deaf05861c424d304829e3fd64a23))
* **deps:** Update module github.com/cloudnative-pg/cnpg-i-machinery to v0.4.1 ([#292](https://github.com/EnterpriseDB/klio/issues/292)) ([6341622](https://github.com/cloudnative-pg/klio/commit/63416224f75ff62a84c0ea7243bf34f25aefa1b6))
* **deps:** Update module sigs.k8s.io/controller-runtime to v0.22.1 ([#248](https://github.com/EnterpriseDB/klio/issues/248)) ([e9489b9](https://github.com/cloudnative-pg/klio/commit/e9489b9c3beac4ca9efd083647db902f44be2fb8))
* Maintain the old service name or everything breaks ([73a5656](https://github.com/cloudnative-pg/klio/commit/73a565642c82471f552216b54662586d5dde6c1e))
* **operator:** Properly update resources ([d460665](https://github.com/cloudnative-pg/klio/commit/d46066549b7ae5ed499be76daf2a4113e2dd3299))
* Replication failure on timeline switch ([#224](https://github.com/EnterpriseDB/klio/issues/224)) ([5504fbe](https://github.com/cloudnative-pg/klio/commit/5504fbeb45453320f4d1cc87032639440b24f0e4))
* Use the same serviceName in sts and service ([fc32410](https://github.com/cloudnative-pg/klio/commit/fc32410e71b920a5d878b047f63c36c48c8210a3))

## [0.0.4](https://github.com/EnterpriseDB/klio/compare/v0.0.3...v0.0.4) (2025-09-08)


### Bug Fixes

* Allow building Klio images on release ([#241](https://github.com/EnterpriseDB/klio/issues/241)) ([435cfb2](https://github.com/cloudnative-pg/klio/commit/435cfb276cdd5b8cbe2811cac47db1bd24898729))

## [0.0.3](https://github.com/EnterpriseDB/klio/compare/v0.0.2...v0.0.3) (2025-09-08)


### Bug Fixes

* Release procedure ([#239](https://github.com/EnterpriseDB/klio/issues/239)) ([b042f4a](https://github.com/cloudnative-pg/klio/commit/b042f4a02e84edd537122d78920e701eca3e4229))

## [0.0.2](https://github.com/EnterpriseDB/klio/compare/v0.0.1...v0.0.2) (2025-09-05)


### Features

* Add backup ([#90](https://github.com/EnterpriseDB/klio/issues/90)) ([390ddee](https://github.com/cloudnative-pg/klio/commit/390ddeed21a60ca4c666a81c1b3107d9c24fd1b6))
* Add cnpgi `metrics` capabilities ([#128](https://github.com/EnterpriseDB/klio/issues/128)) ([ce95a6f](https://github.com/cloudnative-pg/klio/commit/ce95a6f7cf59549745bc7d52e62d1218c7afd587))
* Add retention policies to Klio ([#190](https://github.com/EnterpriseDB/klio/issues/190)) ([0ceb9f7](https://github.com/cloudnative-pg/klio/commit/0ceb9f723e7ce1a3f0fa40341d5fce8443515c8d))
* Add users configuration ([b6e2228](https://github.com/cloudnative-pg/klio/commit/b6e2228dd31c263eb377db6094141064231f1dbe))
* Admin user and password support ([93d3574](https://github.com/cloudnative-pg/klio/commit/93d3574cd66f797075281fd37e70aebc7f12117a))
* Cluster metadata and LSN reset ([6510348](https://github.com/cloudnative-pg/klio/commit/651034804fda6272e5016e200b1cdb5253ffa6c9))
* Remove `ConfigPath` field ([#84](https://github.com/EnterpriseDB/klio/issues/84)) ([ab33fea](https://github.com/cloudnative-pg/klio/commit/ab33fea2b3afb2cca0c531f95459558d92f8f0fa))
* Add persistentVolumeClaim customization ([#85](https://github.com/EnterpriseDB/klio/issues/85)) ([7e178eb](https://github.com/cloudnative-pg/klio/commit/7e178eb3e18ecf3ccdac1902e12debd9f24ed4c9))
* Add kopia AdminUser ([#79](https://github.com/EnterpriseDB/klio/issues/79)) ([f227829](https://github.com/cloudnative-pg/klio/commit/f227829decbeac6cd54590e3ab86a72639920d5e))
* Add imagePullSecrets ([#80](https://github.com/EnterpriseDB/klio/issues/80)) ([445b96f](https://github.com/cloudnative-pg/klio/commit/445b96fca77f4808969cb7359d13abfc9c3883da))
* Add resources ([#81](https://github.com/EnterpriseDB/klio/issues/81)) ([731e1e0](https://github.com/cloudnative-pg/klio/commit/731e1e0727799449dc01d1fa79bc239d5c78e782))
* Add StatefulSet ([#78](https://github.com/EnterpriseDB/klio/issues/78)) ([1296ea9](https://github.com/cloudnative-pg/klio/commit/1296ea963b05d450347b76149104e706594b102e))
* Add CNPGI restore capability ([#121](https://github.com/EnterpriseDB/klio/issues/121)) ([3e41367](https://github.com/cloudnative-pg/klio/commit/3e41367590a0b493948080c4d337d3b5819bf1e0))
* Cnpg-i send-wal cluster coordination ([#191](https://github.com/EnterpriseDB/klio/issues/191)) ([8342951](https://github.com/cloudnative-pg/klio/commit/8342951947d8f73574468e1b3e81b58f88a86f50))
* Create klio core configuration ([07a7208](https://github.com/cloudnative-pg/klio/commit/07a7208a175e382fb941289ed41a69df24cdf5ee))
* Htpasswd-based credential checking ([75ede0a](https://github.com/cloudnative-pg/klio/commit/75ede0adf5e807928d3ddb82a937a8bf14571cfe))
* Initial replica cluster support ([#189](https://github.com/EnterpriseDB/klio/issues/189)) ([40a12a5](https://github.com/cloudnative-pg/klio/commit/40a12a573d148db1048360d9f3a5fd8b91065579))
* Klio backup run/get-metadata ([#123](https://github.com/EnterpriseDB/klio/issues/123)) ([627244a](https://github.com/cloudnative-pg/klio/commit/627244aac219566b261a937a1b03d1eb6b5cfc5b))
* Klio initialize ([732d779](https://github.com/cloudnative-pg/klio/commit/732d7794363ba138ae60befc4eb34e94c8902474))
* Klio server command ([8a443f3](https://github.com/cloudnative-pg/klio/commit/8a443f3226f22e7eb62cead194d9d8f064c3dea4))
* **logs:** Make kopia log to stdout instead of using folder ([#100](https://github.com/EnterpriseDB/klio/issues/100)) ([90ac8a9](https://github.com/cloudnative-pg/klio/commit/90ac8a9f2668e80731866a2ee27eb3b7854e3e3e))
* Operator stub ([#76](https://github.com/EnterpriseDB/klio/issues/76)) ([70a769e](https://github.com/cloudnative-pg/klio/commit/70a769e73d345f0cb408981893fe38a3b5441f1e))
* **operator:** Add CNPGI capabilities ([e0e1191](https://github.com/cloudnative-pg/klio/commit/e0e1191a440d019db3265e217c333a128929dfe9))
* **perf:** Define GRPC window size and buffers ([5d7088b](https://github.com/cloudnative-pg/klio/commit/5d7088b90ba49b2c812c734f7ec0ab0fc6f51bc7))
* **prof:** Pprof server, no protobuf in WAL file blocks ([bf4a689](https://github.com/cloudnative-pg/klio/commit/bf4a6893b1a6cdcff1e775e6fa8563b4f29b2c23))
* Reset LSN ([1e3d150](https://github.com/cloudnative-pg/klio/commit/1e3d150607de59b4e172134cfbf573ca4a416bce))
* **send-wal:** Use a buffer when sending WALs ([#127](https://github.com/EnterpriseDB/klio/issues/127)) ([be5573f](https://github.com/cloudnative-pg/klio/commit/be5573f20a39474a529e06d30384f6c636191c4f))
* Support choosing cluster name in configuration ([#176](https://github.com/EnterpriseDB/klio/issues/176)) ([88949d9](https://github.com/cloudnative-pg/klio/commit/88949d96ba7b2df3ee3b1cc266a52536442717cd))
* **wal-player:** Add WAL file generator and player commands ([#126](https://github.com/EnterpriseDB/klio/issues/126)) ([8e5bb15](https://github.com/cloudnative-pg/klio/commit/8e5bb153e691b0007c21ea7ac7a6625ca5f155e6))
* **wal-player:** Decode GZIP WAL files ([#134](https://github.com/EnterpriseDB/klio/issues/134)) ([f69ee4f](https://github.com/cloudnative-pg/klio/commit/f69ee4fc47a85e07cd4f1a2c82c32795cfeb4d01))


### Bug Fixes

* Append cluster name to GRPC requests ([22574a4](https://github.com/cloudnative-pg/klio/commit/22574a45399a9ee29acdc1c7436ab0a7e8faf6c4))
* Avoid returning errors on successful get-wal ([8f773ef](https://github.com/cloudnative-pg/klio/commit/8f773ef9a67113f518e31dfb347c1104ece763a0))
* Consider grpc overhead while setting message size limit ([#167](https://github.com/EnterpriseDB/klio/issues/167)) ([6c8922e](https://github.com/cloudnative-pg/klio/commit/6c8922ec917bbc15ff4d3a7315ba027f1087a2bb))
* **deps:** Lock file maintenance documentation dependencies ([#152](https://github.com/EnterpriseDB/klio/issues/152)) ([f13f99e](https://github.com/cloudnative-pg/klio/commit/f13f99e2c7826a7c97f3d85a17e38cbb46619743))
* **deps:** Lock file maintenance documentation dependencies ([#164](https://github.com/EnterpriseDB/klio/issues/164)) ([95efba0](https://github.com/cloudnative-pg/klio/commit/95efba03850da2ded1e935fe3cd8e1d2f242fae3))
* **deps:** Lock file maintenance documentation dependencies ([#222](https://github.com/EnterpriseDB/klio/issues/222)) ([b5be1aa](https://github.com/cloudnative-pg/klio/commit/b5be1aa2ccef9473712f79e13f4f7737e8a8f5ff))
* **deps:** Update all non-major go dependencies ([#110](https://github.com/EnterpriseDB/klio/issues/110)) ([4d29c82](https://github.com/cloudnative-pg/klio/commit/4d29c8275f4657ee97d1b4140f84624bcbc99953))
* **deps:** Update all non-major go dependencies ([#155](https://github.com/EnterpriseDB/klio/issues/155)) ([6cf8d04](https://github.com/cloudnative-pg/klio/commit/6cf8d043bdb75c8980a44ea8d07e5140cd195b4e))
* **deps:** Update all non-major go dependencies ([#199](https://github.com/EnterpriseDB/klio/issues/199)) ([6009353](https://github.com/cloudnative-pg/klio/commit/60093532186610d55b6798ea0dfbb2a3c05d0adc))
* **deps:** Update all non-major go dependencies ([#229](https://github.com/EnterpriseDB/klio/issues/229)) ([d788a80](https://github.com/cloudnative-pg/klio/commit/d788a80b541c221a01788bdaa656549e0c387ffb))
* **deps:** Update all non-major go dependencies ([#43](https://github.com/EnterpriseDB/klio/issues/43)) ([3ece96c](https://github.com/cloudnative-pg/klio/commit/3ece96c7accce82ff688142b7493b57e377132f6))
* **deps:** Update all non-major go dependencies ([#70](https://github.com/EnterpriseDB/klio/issues/70)) ([a19d987](https://github.com/cloudnative-pg/klio/commit/a19d9874b50fa6e509a5e2daf612fe580139ec0b))
* **deps:** Update k8s.io/utils digest to 0af2bda ([#212](https://github.com/EnterpriseDB/klio/issues/212)) ([6c086e2](https://github.com/cloudnative-pg/klio/commit/6c086e2a20ab82a56ad370fc96a179ec8de97838))
* **deps:** Update k8s.io/utils digest to 4c0f3b2 ([#103](https://github.com/EnterpriseDB/klio/issues/103)) ([9aef0a6](https://github.com/cloudnative-pg/klio/commit/9aef0a689571c4828fbe3100402508a65c994fd5))
* **deps:** Update kubernetes packages to v0.33.2 ([#111](https://github.com/EnterpriseDB/klio/issues/111)) ([78f9b95](https://github.com/cloudnative-pg/klio/commit/78f9b9520e864406f2a682f81c5285dea55921a5))
* **deps:** Update kubernetes packages to v0.33.3 ([#138](https://github.com/EnterpriseDB/klio/issues/138)) ([b618e8c](https://github.com/cloudnative-pg/klio/commit/b618e8c87a8fd66f734f22471f257177d21e8659))
* **deps:** Update kubernetes packages to v0.33.4 ([#198](https://github.com/EnterpriseDB/klio/issues/198)) ([f7a3071](https://github.com/cloudnative-pg/klio/commit/f7a3071bd3b9bef7ffddf87c52c2dc725ad2a329))
* **deps:** Update module github.com/cloudnative-pg/cloudnative-pg to v1.26.1 ([#162](https://github.com/EnterpriseDB/klio/issues/162)) ([ab8b471](https://github.com/cloudnative-pg/klio/commit/ab8b471f3c93726447fe4ece0108de3ac58f12bc))
* **deps:** Update module github.com/kopia/kopia to v0.21.1 ([#147](https://github.com/EnterpriseDB/klio/issues/147)) ([9572e95](https://github.com/cloudnative-pg/klio/commit/9572e95bc3680a4a305b670863e6827438fe89c7))
* **deps:** Update module golang.org/x/crypto to v0.38.0 ([#64](https://github.com/EnterpriseDB/klio/issues/64)) ([fa4e747](https://github.com/cloudnative-pg/klio/commit/fa4e747741f986d581f5cd6b06f9f7557c134bc5))
* **deps:** Update module sigs.k8s.io/controller-runtime to v0.22.0 ([#220](https://github.com/EnterpriseDB/klio/issues/220)) ([f0743a5](https://github.com/cloudnative-pg/klio/commit/f0743a53caef4581b10b6dd4d837d6917036104c))
* **deps:** Update module sigs.k8s.io/yaml to v1.6.0 ([#200](https://github.com/EnterpriseDB/klio/issues/200)) ([f75a840](https://github.com/cloudnative-pg/klio/commit/f75a840bcc3331cd6c8bb6ab5477a56f3bccb2f8))
* Kopia cache configuration ([e277322](https://github.com/cloudnative-pg/klio/commit/e277322984c8a6e4f2597935daa5ac19d8911deb))
* Prevent wordlist-ordered task to run twice ([b321d85](https://github.com/cloudnative-pg/klio/commit/b321d859d2fc40a609c730bdfaa333c0255eed19))
* Read configuration using environment variables ([27c66d7](https://github.com/cloudnative-pg/klio/commit/27c66d700782f9d3583b1b3453a03d7ddd0d6a05))

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
