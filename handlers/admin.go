package handlers

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"fresh-words-backend/db"
	"fresh-words-backend/models"
	"fresh-words-backend/utils"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type SettingsUpdateRequest struct {
	ChurchName        string `json:"church_name"`
	AppLogoURL        string `json:"app_logo_url"`
	SupportEmail      string `json:"support_email" binding:"omitempty,email"`
	PrivacyPolicyURL  string `json:"privacy_policy_url"`
	TermsOfServiceURL string `json:"terms_of_service_url"`
	AboutUs           string `json:"about_us"`

	DailyDeliveranceEnabled   *bool  `json:"daily_deliverance_enabled"`
	DailyDeliveranceTime      string `json:"daily_deliverance_time"`
	DailyDeliveranceRandomize *bool  `json:"daily_deliverance_randomize"`

	HolinessEnabled   *bool  `json:"holiness_enabled"`
	HolinessTime      string `json:"holiness_time"`
	HolinessRandomize *bool  `json:"holiness_randomize"`

	PrayerEnabled   *bool  `json:"prayer_enabled"`
	PrayerTime      string `json:"prayer_time"`
	PrayerRandomize *bool  `json:"prayer_randomize"`

	YearlyDevotionalEnabled   *bool  `json:"yearly_devotional_enabled"`
	YearlyDevotionalTime      string `json:"yearly_devotional_time"`
	YearlyDevotionalRandomize *bool  `json:"yearly_devotional_randomize"`

	DailyQuoteText    string `json:"daily_quote_text"`
	DailyQuoteAuthor  string `json:"daily_quote_author"`
}

// GetDashboardStatsHandler returns count statistics for the admin control panel.
func GetDashboardStatsHandler(c *gin.Context) {
	var totalFeedback int64
	var unreadFeedback int64
	var publishedPackages int64
	var totalDevotionals int64

	db.DB.Model(&models.Feedback{}).Count(&totalFeedback)
	db.DB.Model(&models.Feedback{}).Where("is_read = ?", false).Count(&unreadFeedback)
	db.DB.Model(&models.Package{}).Where("status = ?", "published").Count(&publishedPackages)
	db.DB.Model(&models.Devotional{}).Count(&totalDevotionals)

	var activePackages []models.Package
	err := db.DB.Preload("Devotionals").Where("status = ?", "published").Find(&activePackages).Error
	if err != nil {
		activePackages = []models.Package{}
	}

	// Dynamic Active Reads metrics
	var totalActiveReads int64
	db.DB.Model(&models.DevotionalRead{}).Count(&totalActiveReads)

	// Calculate active reads percentage change vs last week
	now := time.Now()
	startOfThisWeek := now.AddDate(0, 0, -7)
	startOfLastWeek := now.AddDate(0, 0, -14)

	var readsThisWeek int64
	var readsLastWeek int64

	db.DB.Model(&models.DevotionalRead{}).Where("read_at >= ?", startOfThisWeek).Count(&readsThisWeek)
	db.DB.Model(&models.DevotionalRead{}).Where("read_at >= ? AND read_at < ?", startOfLastWeek, startOfThisWeek).Count(&readsLastWeek)

	percentageChangeStr := "0% vs last week"
	if readsLastWeek > 0 {
		pct := (float64(readsThisWeek-readsLastWeek) / float64(readsLastWeek)) * 100
		sign := ""
		if pct > 0 {
			sign = "↑ "
		} else if pct < 0 {
			sign = "↓ "
		}
		percentageChangeStr = fmt.Sprintf("%s%.1f%% vs last week", sign, pct)
	} else if readsThisWeek > 0 {
		percentageChangeStr = "↑ 100% vs last week"
	}

	// Generate daily analytics for the past 14 days (index 0 is 13 days ago, index 13 is today)
	type DailyStat struct {
		Day      string `json:"day"`
		Reads    int64  `json:"reads"`
		Feedback int64  `json:"feedback"`
	}

	dailyAnalytics := make([]DailyStat, 14)
	for i := 0; i < 14; i++ {
		d := now.AddDate(0, 0, -13+i)
		dayLabel := d.Format("Jan 02") // e.g. "Jul 08"

		startOfDay := time.Date(d.Year(), d.Month(), d.Day(), 0, 0, 0, 0, d.Location())
		endOfDay := startOfDay.AddDate(0, 0, 1)

		var readsCount int64
		var feedbackCount int64

		db.DB.Model(&models.DevotionalRead{}).Where("read_at >= ? AND read_at < ?", startOfDay, endOfDay).Count(&readsCount)
		db.DB.Model(&models.Feedback{}).Where("created_at >= ? AND created_at < ?", startOfDay, endOfDay).Count(&feedbackCount)

		dailyAnalytics[i] = DailyStat{
			Day:      dayLabel,
			Reads:    readsCount,
			Feedback: feedbackCount,
		}
	}

	stats := gin.H{
		"total_feedback":                  totalFeedback,
		"unread_feedback":                 unreadFeedback,
		"published_packages":              publishedPackages,
		"total_devotionals":               totalDevotionals,
		"db_storage_usage":                "4.2 MB",
		"active_packages":                 activePackages,
		"total_active_reads":              totalActiveReads,
		"active_reads_percentage_change": percentageChangeStr,
		"daily_analytics":                 dailyAnalytics,
	}

	utils.SendSuccess(c, http.StatusOK, "Dashboard statistics retrieved successfully", stats)
}

// GetFeedbackHandler returns a paginated list of congregation feedback messages.
func GetFeedbackHandler(c *gin.Context) {
	pageStr := c.DefaultQuery("page", "1")
	limitStr := c.DefaultQuery("limit", "10")

	page, err1 := strconv.Atoi(pageStr)
	limit, err2 := strconv.Atoi(limitStr)
	if err1 != nil || err2 != nil || page < 1 || limit < 1 {
		utils.SendError(c, http.StatusBadRequest, "Invalid pagination page or limit parameters", nil)
		return
	}

	var total int64
	db.DB.Model(&models.Feedback{}).Count(&total)

	var feedbacks []models.Feedback
	offset := (page - 1) * limit
	err := db.DB.Order("created_at desc").Limit(limit).Offset(offset).Find(&feedbacks).Error
	if err != nil {
		utils.SendError(c, http.StatusInternalServerError, "Failed to query feedback list", err.Error())
		return
	}

	utils.SendPaginated(c, http.StatusOK, "Feedback list retrieved", feedbacks, total, page, limit)
}

// MarkFeedbackReadHandler marks a feedback submission as read.
func MarkFeedbackReadHandler(c *gin.Context) {
	idStr := c.Param("id")
	feedbackID, err := uuid.Parse(idStr)
	if err != nil {
		utils.SendError(c, http.StatusBadRequest, "Invalid feedback ID format", err.Error())
		return
	}

	var feedback models.Feedback
	err = db.DB.First(&feedback, "id = ?", feedbackID).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			utils.SendError(c, http.StatusNotFound, "Feedback item not found", nil)
			return
		}
		utils.SendError(c, http.StatusInternalServerError, "Query failed", err.Error())
		return
	}

	feedback.IsRead = true
	if err := db.DB.Save(&feedback).Error; err != nil {
		utils.SendError(c, http.StatusInternalServerError, "Failed to update feedback status", err.Error())
		return
	}

	utils.SendSuccess(c, http.StatusOK, "Feedback marked as read successfully", feedback)
}

// DeleteFeedbackHandler deletes a specific feedback submission.
func DeleteFeedbackHandler(c *gin.Context) {
	idStr := c.Param("id")
	feedbackID, err := uuid.Parse(idStr)
	if err != nil {
		utils.SendError(c, http.StatusBadRequest, "Invalid feedback ID format", err.Error())
		return
	}

	var feedback models.Feedback
	err = db.DB.First(&feedback, "id = ?", feedbackID).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			utils.SendError(c, http.StatusNotFound, "Feedback item not found", nil)
			return
		}
		utils.SendError(c, http.StatusInternalServerError, "Query failed", err.Error())
		return
	}

	if err := db.DB.Delete(&feedback).Error; err != nil {
		utils.SendError(c, http.StatusInternalServerError, "Failed to delete feedback", err.Error())
		return
	}

	utils.SendSuccess(c, http.StatusOK, "Feedback deleted successfully", nil)
}

// GetSettingsHandler retrieves active church branding settings (seeding defaults if none exist).
func GetSettingsHandler(c *gin.Context) {
	var settings models.Settings
	err := db.DB.First(&settings).Error

	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			// Seed default system settings
			settings = models.Settings{
				ChurchName:        "Fresh Words Devotional",
				SupportEmail:      "info@freshwords.org",
				PrivacyPolicyURL:  "https://freshdevotionals.com/privacy",
				TermsOfServiceURL: "https://freshdevotionals.com/terms",
				AboutUs:           "Growing with God every single day. Daily deliverance, holiness, prayer and yearly devotionals.",
				
				DailyDeliveranceEnabled:   true,
				DailyDeliveranceTime:      "08:00 AM",
				DailyDeliveranceRandomize: true,

				HolinessEnabled:   true,
				HolinessTime:      "09:00 AM",
				HolinessRandomize: true,

				PrayerEnabled:   true,
				PrayerTime:      "08:30 AM",
				PrayerRandomize: true,

				YearlyDevotionalEnabled:   true,
				YearlyDevotionalTime:      "08:00 AM",
				YearlyDevotionalRandomize: false,

				DailyQuoteText:    "Now faith is the assurance of things hoped for, the conviction of things not seen.",
				DailyQuoteAuthor:  "Hebrews 11:1",
			}
			if createErr := db.DB.Create(&settings).Error; createErr != nil {
				utils.SendError(c, http.StatusInternalServerError, "Failed to seed default settings", createErr.Error())
				return
			}
			utils.SendSuccess(c, http.StatusOK, "Default settings initialized", settings)
			return
		}
		utils.SendError(c, http.StatusInternalServerError, "Failed to load settings", err.Error())
		return
	}

	utils.SendSuccess(c, http.StatusOK, "Settings retrieved successfully", settings)
}

// UpdateSettingsHandler updates the church metadata configurations.
func UpdateSettingsHandler(c *gin.Context) {
	var req SettingsUpdateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.SendError(c, http.StatusBadRequest, "Invalid settings input formats", err.Error())
		return
	}

	var settings models.Settings
	err := db.DB.First(&settings).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		utils.SendError(c, http.StatusInternalServerError, "Database error", err.Error())
		return
	}

	settings.ChurchName = req.ChurchName
	settings.AppLogoURL = req.AppLogoURL
	settings.SupportEmail = req.SupportEmail
	settings.PrivacyPolicyURL = req.PrivacyPolicyURL
	settings.TermsOfServiceURL = req.TermsOfServiceURL
	settings.AboutUs = req.AboutUs

	if req.DailyDeliveranceEnabled != nil {
		settings.DailyDeliveranceEnabled = *req.DailyDeliveranceEnabled
	}
	if req.DailyDeliveranceTime != "" {
		settings.DailyDeliveranceTime = req.DailyDeliveranceTime
	}
	if req.DailyDeliveranceRandomize != nil {
		settings.DailyDeliveranceRandomize = *req.DailyDeliveranceRandomize
	}

	if req.HolinessEnabled != nil {
		settings.HolinessEnabled = *req.HolinessEnabled
	}
	if req.HolinessTime != "" {
		settings.HolinessTime = req.HolinessTime
	}
	if req.HolinessRandomize != nil {
		settings.HolinessRandomize = *req.HolinessRandomize
	}

	if req.PrayerEnabled != nil {
		settings.PrayerEnabled = *req.PrayerEnabled
	}
	if req.PrayerTime != "" {
		settings.PrayerTime = req.PrayerTime
	}
	if req.PrayerRandomize != nil {
		settings.PrayerRandomize = *req.PrayerRandomize
	}

	if req.YearlyDevotionalEnabled != nil {
		settings.YearlyDevotionalEnabled = *req.YearlyDevotionalEnabled
	}
	if req.YearlyDevotionalTime != "" {
		settings.YearlyDevotionalTime = req.YearlyDevotionalTime
	}
	if req.YearlyDevotionalRandomize != nil {
		settings.YearlyDevotionalRandomize = *req.YearlyDevotionalRandomize
	}

	if req.DailyQuoteText != "" {
		settings.DailyQuoteText = req.DailyQuoteText
	}
	if req.DailyQuoteAuthor != "" {
		settings.DailyQuoteAuthor = req.DailyQuoteAuthor
	}

	var saveErr error
	if settings.ID == uuid.Nil {
		settings.ID = uuid.New()
		saveErr = db.DB.Create(&settings).Error
	} else {
		saveErr = db.DB.Save(&settings).Error
	}

	if saveErr != nil {
		utils.SendError(c, http.StatusInternalServerError, "Failed to save settings modifications", saveErr.Error())
		return
	}

	utils.SendSuccess(c, http.StatusOK, "Settings saved successfully", settings)
}
