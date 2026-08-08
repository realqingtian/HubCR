package jobstore

import "time"

type jobRecord struct {
	ID             string     `gorm:"column:id"`
	Kind           string     `gorm:"column:kind"`
	IntentKey      string     `gorm:"column:intent_key"`
	Payload        []byte     `gorm:"column:payload"`
	State          string     `gorm:"column:state"`
	AttemptCount   int        `gorm:"column:attempt_count"`
	MaxAttempts    int        `gorm:"column:max_attempts"`
	AvailableAt    time.Time  `gorm:"column:available_at"`
	LeaseOwner     *string    `gorm:"column:lease_owner"`
	LeaseExpiresAt *time.Time `gorm:"column:lease_expires_at"`
	LastErrorCode  *string    `gorm:"column:last_error_code"`
	CreatedAt      time.Time  `gorm:"column:created_at"`
	UpdatedAt      time.Time  `gorm:"column:updated_at"`
	StartedAt      *time.Time `gorm:"column:started_at"`
	CompletedAt    *time.Time `gorm:"column:completed_at"`
}

func (jobRecord) TableName() string { return "jobs" }
