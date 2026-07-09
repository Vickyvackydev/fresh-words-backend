package models

import (
	"time"

	"github.com/google/uuid"
)

type DevotionalSchedule struct {
	ID           uuid.UUID  `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	Year         int        `gorm:"not null;index:idx_year_category_day,unique" json:"year"`
	Category     string     `gorm:"type:varchar(50);not null;index:idx_year_category_day,unique" json:"category"`
	DayOfYear    int        `gorm:"not null;index:idx_year_category_day,unique" json:"day_of_year"` // 1-366
	DevotionalID uuid.UUID  `gorm:"type:uuid;not null;index" json:"devotional_id"`
	Devotional   Devotional `gorm:"foreignKey:DevotionalID" json:"devotional"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
}
