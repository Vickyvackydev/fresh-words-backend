package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// ContentTranslation caches AI translations of devotionals and hymns
type ContentTranslation struct {
	ID             uuid.UUID      `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	ContentType    string         `gorm:"index:idx_content_trans,unique;type:varchar(50);not null" json:"content_type"` // "devotional" | "hymn"
	ContentID      string         `gorm:"index:idx_content_trans,unique;type:varchar(100);not null" json:"content_id"`   // devotional UUID or hymn ID
	Language       string         `gorm:"index:idx_content_trans,unique;type:varchar(10);not null" json:"language"`     // "fr", "es", "yo", "ig", "ha", "pt"
	TranslatedData string         `gorm:"type:text;not null" json:"translated_data"`                                     // JSON payload with translated fields
	CreatedAt      time.Time      `json:"created_at"`
	UpdatedAt      time.Time      `json:"updated_at"`
	DeletedAt      gorm.DeletedAt `gorm:"index" json:"-"`
}

func (ct *ContentTranslation) BeforeCreate(tx *gorm.DB) error {
	if ct.ID == uuid.Nil {
		ct.ID = uuid.New()
	}
	return nil
}
