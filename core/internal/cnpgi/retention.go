package cnpgi

import (
	"fmt"

	"github.com/spf13/afero"
	"sigs.k8s.io/yaml"

	"github.com/cloudnative-pg/klio/core/pkg/config"
)

// Retention contains the retention policy configuration.
type Retention struct {
	KeepLatest  *int
	KeepAnnual  *int
	KeepMonthly *int
	KeepWeekly  *int
	KeepDaily   *int
	KeepHourly  *int
}

// IsEmpty checks if the retention configuration is empty or not.
func (r *Retention) IsEmpty() bool {
	if r == nil {
		return true
	}

	emptyRetention := Retention{}

	return *r == emptyRetention
}

// extractRetentionFromConfiguration reads retention policy settings.
func extractRetentionFromConfiguration() (*Retention, error) {
	osFS := afero.NewOsFs()
	f, err := afero.ReadFile(osFS, backupRepositoryConfigPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read backup repository config file %q: %w", backupRepositoryConfigPath, err)
	}

	var configData config.Data
	err = yaml.Unmarshal(f, &configData)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal backup repository config file %q: %w", backupRepositoryConfigPath, err)
	}

	conf := Retention{}

	if configData.RetentionPolicy == nil {
		return &conf, nil
	}

	conf.KeepLatest = configData.RetentionPolicy.KeepLatest
	conf.KeepAnnual = configData.RetentionPolicy.KeepAnnual
	conf.KeepMonthly = configData.RetentionPolicy.KeepMonthly
	conf.KeepWeekly = configData.RetentionPolicy.KeepWeekly
	conf.KeepDaily = configData.RetentionPolicy.KeepDaily
	conf.KeepHourly = configData.RetentionPolicy.KeepHourly

	return &conf, nil
}
