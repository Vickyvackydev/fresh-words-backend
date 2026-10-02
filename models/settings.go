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

	DailyQuoteText    string         `gorm:"type:text;not null;default:'Now faith is the assurance of things hoped for, the conviction of things not seen.'" json:"daily_quote_text"`
	DailyQuoteAuthor  string         `gorm:"type:varchar(255);not null;default:'Hebrews 11:1'" json:"daily_quote_author"`

	LatestAppVersion   string `gorm:"type:varchar(50);not null;default:'1.0.3'" json:"latest_app_version"`
	MinRequiredVersion string `gorm:"type:varchar(50);not null;default:'1.0.0'" json:"min_required_version"`
	ForceUpdate        bool   `gorm:"type:boolean;not null;default:false" json:"force_update"`
	UpdateTitle        string `gorm:"type:varchar(255);not null;default:'Update Available'" json:"update_title"`
	UpdateMessage      string `gorm:"type:text;not null;default:'A new version of Fresh Devotionals is available with improved translations, bible features, and performance enhancements.'" json:"update_message"`
	PlayStoreURL       string `gorm:"type:text;default:'https://play.google.com/store/apps/details?id=com.freshdevotionals.app'" json:"play_store_url"`
	AppStoreURL        string `gorm:"type:text;default:'https://apps.apple.com/app/fresh-devotionals/id6742352824'" json:"app_store_url"`

	CreatedAt         time.Time      `json:"created_at"`
	UpdatedAt         time.Time      `json:"updated_at"`
	DeletedAt         gorm.DeletedAt `gorm:"index" json:"-"`
}
