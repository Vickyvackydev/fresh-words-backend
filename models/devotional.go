package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type Devotional struct {
	ID                 uuid.UUID      `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	PackageID          uuid.UUID      `gorm:"type:uuid;not null;index" json:"package_id"`
	Category           string         `gorm:"type:varchar(50);not null;index" json:"category"` // daily_deliverance, holiness, prayer, yearly
	Title              string         `gorm:"type:text;not null" json:"title"`
	ScriptureQuote     string         `gorm:"type:text;not null" json:"scripture_quote"`
	ScriptureReference string         `gorm:"type:text;not null" json:"scripture_reference"`
	Body               string         `gorm:"type:text;not null" json:"body"`
	Prayer             string         `gorm:"type:text" json:"prayer,omitempty"`
	Reflection         string         `gorm:"type:text" json:"reflection,omitempty"`
	ActionPoints       string         `gorm:"type:text" json:"action_points,omitempty"` // Serialized JSON array of strings
	DefaultDay         int            `gorm:"not null;index" json:"default_day"` // 1-366 indicating day of the year in document
	CreatedAt          time.Time      `json:"created_at"`
	UpdatedAt          time.Time      `json:"updated_at"`
	DeletedAt          gorm.DeletedAt `gorm:"index" json:"-"`
}
