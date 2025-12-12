package kopia

// Policy describes snapshot policy for a single source.
type Policy struct {
	Labels          map[string]string `json:"-"`
	RetentionPolicy RetentionPolicy   `json:"retention"`
	NoParent        bool              `json:"noParent,omitempty"`
}

// RetentionPolicy describes snapshot retention policy.
type RetentionPolicy struct {
	KeepLatest  *int `json:"keepLatest,omitempty"`
	KeepHourly  *int `json:"keepHourly,omitempty"`
	KeepDaily   *int `json:"keepDaily,omitempty"`
	KeepWeekly  *int `json:"keepWeekly,omitempty"`
	KeepMonthly *int `json:"keepMonthly,omitempty"`
	KeepAnnual  *int `json:"keepAnnual,omitempty"`
}
