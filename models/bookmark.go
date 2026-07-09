package models

import (
	"time"

	"github.com/google/uuid"
)

type UserBookmark struct {
	ID           uuid.UUID  `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	DeviceID     string     `gorm:"type:varchar(255);not null;index:idx_device_devo,unique" json:"device_id"`
	DevotionalID uuid.UUID  `gorm:"type:uuid;not null;index:idx_device_devo,unique" json:"devotional_id"`
	Devotional   Devotional `gorm:"foreignKey:DevotionalID;constraint:OnDelete:CASCADE" json:"devotional,omitempty"`
	CreatedAt    time.Time  `json:"created_at"`
}