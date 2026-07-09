package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type Package struct {
	ID         uuid.UUID      `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	Category   string         `gorm:"type:varchar(50);not null;index" json:"category"`
	Year       int            `gorm:"not null;index" json:"year"`
	Status     string         `gorm:"type:varchar(20);not null;default:'draft';index" json:"status"` // draft, published, archived
	FileName   string         `gorm:"type:varchar(255);not null" json:"file_name"`
	UploadedAt time.Time      `json:"uploaded_at"`
	CreatedAt  time.Time      `json:"created_at"`
	UpdatedAt  time.Time      `json:"updated_at"`
	DeletedAt  gorm.DeletedAt `gorm:"index" json:"-"`
	Devotionals []Devotional  `gorm:"foreignKey:PackageID;constraint:OnDelete:CASCADE" json:"devotionals,omitempty"`
}
