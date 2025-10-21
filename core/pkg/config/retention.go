package config

// RetentionPolicy defines how many backups we should keep.
type RetentionPolicy struct {
	// KeepLatest is the number of latest backups to keep
	KeepLatest *int `json:"keepLatest,omitempty" mapstructure:"keepLatest"`

	// KeepAnnual is the number of annual backups to keep
	KeepAnnual *int `json:"keepAnnual,omitempty" mapstructure:"keepAnnual"`

	// KeepMonthly is the number of monthly backups to keep
	KeepMonthly *int `json:"keepMonthly,omitempty" mapstructure:"keepMonthly"`

	// KeepWeekly is the number of weekly backups to keep
	KeepWeekly *int `json:"keepWeekly,omitempty" mapstructure:"keepWeekly"`

	// KeepDaily is the number of daily backups to keep
	KeepDaily *int `json:"keepDaily,omitempty" mapstructure:"keepDaily"`

	// KeepHourly is the number of hourly backups to keep
	KeepHourly *int `json:"keepHourly,omitempty" mapstructure:"keepHourly"`
}
