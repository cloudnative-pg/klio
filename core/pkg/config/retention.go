package config

// RetentionPolicy defines how many backups we should keep.
type RetentionPolicy struct {
	// KeepLatest is the number of latest backups to keep
	// optional
	KeepLatest *int `json:"keepLatest,omitempty" mapstructure:"keepLatest"`

	// KeepAnnual is the number of annual backups to keep
	// optional
	KeepAnnual *int `json:"keepAnnual,omitempty" mapstructure:"keepAnnual"`

	// KeepMonthly is the number of monthly backups to keep
	// optional
	KeepMonthly *int `json:"keepMonthly,omitempty" mapstructure:"keepMonthly"`

	// KeepWeekly is the number of weekly backups to keep
	// optional
	KeepWeekly *int `json:"keepWeekly,omitempty" mapstructure:"keepWeekly"`

	// KeepDaily is the number of daily backups to keep
	// optional
	KeepDaily *int `json:"keepDaily,omitempty" mapstructure:"keepDaily"`

	// KeepHourly is the number of hourly backups to keep
	// optional
	KeepHourly *int `json:"keepHourly,omitempty" mapstructure:"keepHourly"`
}
