package models

import (
	"time"

	"github.com/google/uuid"
)

type DevotionalRead struct {
	ID           uuid.UUID  `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	DevotionalID uuid.UUID  `gorm:"type:uuid;not null;index" json:"devotional_id"`
	Devotional   Devotional `gorm:"foreignKey:DevotionalID;constraint:OnDelete:CASCADE" json:"devotional,omitempty"`
	ReadAt       time.Time  `gorm:"not null;default:now()" json:"read_at"`
}