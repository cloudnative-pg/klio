# Changelog

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
