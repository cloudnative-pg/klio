package config

// RetentionPolicy defines how many backups we should keep.
type RetentionPolicy struct {
	// KeepLatest is the number of latest backups to keep
	KeepLatest *int `json:"keep_latest,omitempty" mapstructure:"keep_latest"`

	// KeepAnnual is the number of annual backups to keep
	KeepAnnual *int `json:"keep_annual,omitempty" mapstructure:"keep_annual"`

	// KeepMonthly is the number of monthly backups to keep
	KeepMonthly *int `json:"keep_monthly,omitempty" mapstructure:"keep_monthly"`

	// KeepWeekly is the number of weekly backups to keep
	KeepWeekly *int `json:"keep_weekly,omitempty" mapstructure:"keep_weekly"`

	// KeepDaily is the number of daily backups to keep
	KeepDaily *int `json:"keep_daily,omitempty" mapstructure:"keep_daily"`

	// KeepHourly is the number of hourly backups to keep
	KeepHourly *int `json:"keep_hourly,omitempty" mapstructure:"keep_hourly"`
}
