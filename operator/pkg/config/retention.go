package config

// RetentionPolicy defines how many backups we should keep.
type RetentionPolicy struct {
	// KeepLatest is the number of latest backups to keep
	KeepLatest *int `json:"keep_latest,omitempty" mapstructure:"keepLatest"`

	// KeepAnnual is the number of annual backups to keep
	KeepAnnual *int `json:"keep_annual,omitempty" mapstructure:"keepAnnual"`

	// KeepMonthly is the number of monthly backups to keep
	KeepMonthly *int `json:"keep_monthly,omitempty" mapstructure:"keepMonthly"`

	// KeepWeekly is the number of weekly backups to keep
	KeepWeekly *int `json:"keep_weekly,omitempty" mapstructure:"keepWeekly"`

	// KeepDaily is the number of daily backups to keep
	KeepDaily *int `json:"keep_daily,omitempty" mapstructure:"keepDaily"`

	// KeepHourly is the number of hourly backups to keep
	KeepHourly *int `json:"keep_hourly,omitempty" mapstructure:"keepHourly"`
}
