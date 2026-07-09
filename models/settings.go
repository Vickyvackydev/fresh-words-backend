package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type Settings struct {
	ID                uuid.UUID      `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	ChurchName        string         `gorm:"type:varchar(255);not null;default:'Fresh Words Devotional'" json:"church_name"`
	AppLogoURL        string         `gorm:"type:text" json:"app_logo_url"`
	SupportEmail      string         `gorm:"type:varchar(255);not null;default:'support@freshwords.org'" json:"support_email"`
	PrivacyPolicyURL  string         `gorm:"type:text" json:"privacy_policy_url"`
	TermsOfServiceURL string         `gorm:"type:text" json:"terms_of_service_url"`
	AboutUs           string         `gorm:"type:text" json:"about_us"`

	DailyDeliveranceEnabled   bool   `gorm:"type:boolean;not null;default:true" json:"daily_deliverance_enabled"`
	DailyDeliveranceTime      string `gorm:"type:varchar(10);not null;default:'08:00 AM'" json:"daily_deliverance_time"`
	DailyDeliveranceRandomize bool   `gorm:"type:boolean;not null;default:true" json:"daily_deliverance_randomize"`

	HolinessEnabled   bool   `gorm:"type:boolean;not null;default:true" json:"holiness_enabled"`
	HolinessTime      string `gorm:"type:varchar(10);not null;default:'09:00 AM'" json:"holiness_time"`
	HolinessRandomize bool   `gorm:"type:boolean;not null;default:true" json:"holiness_randomize"`

	PrayerEnabled   bool   `gorm:"type:boolean;not null;default:true" json:"prayer_enabled"`
	PrayerTime      string `gorm:"type:varchar(10);not null;default:'08:30 AM'" json:"prayer_time"`
	PrayerRandomize bool   `gorm:"type:boolean;not null;default:true" json:"prayer_randomize"`

	YearlyDevotionalEnabled   bool   `gorm:"type:boolean;not null;default:true" json:"yearly_devotional_enabled"`
	YearlyDevotionalTime      string `gorm:"type:varchar(10);not null;default:'08:00 AM'" json:"yearly_devotional_time"`
	YearlyDevotionalRandomize bool   `gorm:"type:boolean;not null;default:false" json:"yearly_devotional_randomize"`

	CreatedAt         time.Time      `json:"created_at"`
	UpdatedAt         time.Time      `json:"updated_at"`
	DeletedAt         gorm.DeletedAt `gorm:"index" json:"-"`
}
