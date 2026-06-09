package e2e

import (
	"time"

	machineryFeatures "github.com/cloudnative-pg/klio/operator/test/machinery/pkg/features"
)

// NewTier2PitrFeatureConfig creates a new tier2 PITR feature configuration.
// It reuses machinery's PitrFeature and injects tier2-specific logic via MutateRecoveryCluster.
func NewTier2PitrFeatureConfig(
	name string, instances int, namespace string,
) machineryFeatures.RecoveryFeatureConfig {
	// Build all resources using the shared builder
	res := buildTier2ScenarioResources(namespace, instances)

	// Create scenario from shared resources
	scenario := &tier2RecoveryScenario{
		namespace:                       res.Namespace,
		issuer:                          res.Issuer,
		rustfsSecret:                    res.RustfsSecret,
		rustfsConfigMap:                 res.RustfsConfigMap,
		rustfsCertificate:               res.RustfsCertificate,
		rustfsService:                   res.RustfsService,
		rustfsDeployment:                res.RustfsDeployment,
		rustfsCreateBucketJob:           res.RustfsCreateBucketJob,
		serverCertificate:               res.ServerCertificate,
		caCertificate:                   res.CACertificate,
		caIssuer:                        res.CAIssuer,
		userCertificate:                 res.UserCertificate,
		encryptionSecret:                res.EncryptionSecret,
		identitySecret:                  res.IdentitySecret,
		klioServer:                      res.KlioServer,
		cnpgCluster:                     res.CNPGCluster,
		klioPluginConfigurationSource:   res.KlioPluginConfigurationSource,
		recoveryServerCertificate:       res.RecoveryServerCertificate,
		recoveryServerCACertificate:     res.RecoveryServerCACertificate,
		recoveryServerCAIssuer:          res.RecoveryServerCAIssuer,
		recoveryUserCertificate:         res.RecoveryUserCertificate,
		recoveryServer:                  res.RecoveryServer,
		klioPluginConfigurationRecovery: res.KlioPluginConfigurationRecovery,
		name:                            name,
	}

	// Use machinery's PitrFeature with tier2-specific mutator
	return machineryFeatures.RecoveryFeatureConfig{
		Name:             name,
		Setup:            scenario.Setup,
		Teardown:         scenario.Teardown,
		SourcePrimaryPod: &scenario.sourcePrimaryPod,
		Backup:           res.Backup,
		RecoveryCluster:  res.RecoveryCluster,
		// Inject tier2-specific logic: wait for tier2 replication + deploy recovery server
		MutateRecoveryCluster: []machineryFeatures.RecoveryClusterMutateFunc{scenario.deployRecoveryServer},
		BackupTimeout:         5 * time.Minute,
	}
}

// RecoverClusterFromTier2Pitr returns a PitrFeature for tier2 PITR recovery testing.
// It reuses machinery's PitrFeature which handles all PITR logic (backup, insert data,
// capture timestamp, drop data, set recovery target, recover, verify).
// The tier2-specific part (wait for tier2 replication + deploy recovery server) is
// injected via MutateRecoveryCluster.
func RecoverClusterFromTier2Pitr(namespace string) *machineryFeatures.PitrFeature {
	return machineryFeatures.NewPitrFeature(
		NewTier2PitrFeatureConfig("RecoverClusterFromTier2Pitr", 1, namespace))
}
