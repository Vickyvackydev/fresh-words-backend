package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type Hymn struct {
	ID        uuid.UUID      `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	Number    int            `gorm:"not null;uniqueIndex" json:"number"`
	Title     string         `gorm:"type:text;not null;index" json:"title"`
	Chorus    string         `gorm:"type:text" json:"chorus,omitempty"`
	Verses    string         `gorm:"type:text;not null" json:"verses"` // JSON string array of stanzas
	Category  string         `gorm:"type:text;index" json:"category,omitempty"`
	Author    string         `gorm:"type:text" json:"author,omitempty"`
	Key       string         `gorm:"type:text" json:"key,omitempty"` // Musical Key or Meter
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`
}

func (h *Hymn) BeforeCreate(tx *gorm.DB) error {
	if h.ID == uuid.Nil {
		h.ID = uuid.New()
	}
	return nil
}
