package handlers

import (
	"errors"
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

type FeedbackRequest struct {
	Name    string `json:"name" binding:"required"`
	Email   string `json:"email" binding:"required,email"`
	Message string `json:"message" binding:"required"`
}

// GetTodayDevotionalHandler gets the scheduled devotional for the client's current date.
func GetTodayDevotionalHandler(c *gin.Context) {
	category := c.Query("category")
	timezone := c.Query("timezone")

	if category == "" {
		utils.SendError(c, http.StatusBadRequest, "Category parameter is required", nil)
		return
	}

	// Calculate client's current date based on timezone
	var clientTime time.Time
	if timezone != "" {
		loc, err := time.LoadLocation(timezone)
		if err == nil {
			clientTime = time.Now().In(loc)
		} else {
			clientTime = time.Now().UTC()
		}
	} else {
		clientTime = time.Now().UTC()
	}

	year := clientTime.Year()
	dayOfYear := clientTime.YearDay()

	var schedule models.DevotionalSchedule
	err := db.DB.Preload("Devotional").
		Where("year = ? AND category = ? AND day_of_year = ?", year, category, dayOfYear).
		First(&schedule).Error

	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			// If not found, fall back to default day 1 of the latest published package
			var devotional models.Devotional
			fallbackErr := db.DB.Joins("JOIN packages ON packages.id = devotionals.package_id").
				Where("packages.status = 'published' AND devotionals.category = ?", category).
				Order("devotionals.default_day asc").
				First(&devotional).Error

			if fallbackErr != nil {
				utils.SendError(c, http.StatusNotFound, "No devotionals published or scheduled for this date", nil)
				return
			}
			utils.SendSuccess(c, http.StatusOK, "No active schedule found; returned fallback day 1", devotional)
			return
		}

		utils.SendError(c, http.StatusInternalServerError, "Database query failed", err.Error())
		return
	}

	utils.SendSuccess(c, http.StatusOK, "Today's devotional retrieved successfully", schedule.Devotional)
}

// GetDevotionalByDateHandler gets the scheduled devotional for a specific calendar date (YYYY-MM-DD).
func GetDevotionalByDateHandler(c *gin.Context) {
	category := c.Query("category")
	dateStr := c.Query("date")

	if category == "" || dateStr == "" {
		utils.SendError(c, http.StatusBadRequest, "Category and date parameters are required", nil)
		return
	}

	parsedDate, err := time.Parse("2006-01-02", dateStr)
	if err != nil {
		utils.SendError(c, http.StatusBadRequest, "Invalid date format, expected YYYY-MM-DD", err.Error())
		return
	}

	year := parsedDate.Year()
	dayOfYear := parsedDate.YearDay()

	var schedule models.DevotionalSchedule
	err = db.DB.Preload("Devotional").
		Where("year = ? AND category = ? AND day_of_year = ?", year, category, dayOfYear).
		First(&schedule).Error

	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			utils.SendError(c, http.StatusNotFound, "No devotional scheduled for this date", nil)
			return
		}
		utils.SendError(c, http.StatusInternalServerError, "Database query failed", err.Error())
		return
	}

	utils.SendSuccess(c, http.StatusOK, "Devotional retrieved successfully", schedule.Devotional)
}

// GetCalendarDevotionalsHandler returns daily titles and scheduled states for a month grid.
func GetCalendarDevotionalsHandler(c *gin.Context) {
	category := c.Query("category")
	yearStr := c.Query("year")
	monthStr := c.Query("month")

	if category == "" || yearStr == "" || monthStr == "" {
		utils.SendError(c, http.StatusBadRequest, "Category, year, and month parameters are required", nil)
		return
	}

	year, err1 := strconv.Atoi(yearStr)
	month, err2 := strconv.Atoi(monthStr)
	if err1 != nil || err2 != nil || month < 1 || month > 12 {
		utils.SendError(c, http.StatusBadRequest, "Invalid year or month format", nil)
		return
	}

	// Calculate starting and ending day-of-year indices for the target month
	firstOfMonth := time.Date(year, time.Month(month), 1, 0, 0, 0, 0, time.UTC)
	lastOfMonth := firstOfMonth.AddDate(0, 1, -1)

	startDay := firstOfMonth.YearDay()
	endDay := lastOfMonth.YearDay()

	// Special case: if lastOfMonth wraps to next year (shouldn't, but safety check)
	if lastOfMonth.Year() > year {
		endDay = 365
		if year%4 == 0 && (year%100 != 0 || year%400 == 0) {
			endDay = 366
		}
	}

	type CalendarItem struct {
		DayOfYear int    `json:"day_of_year"`
		Date      string `json:"date"`
		Title     string `json:"title"`
		Done      bool   `json:"done"` // Client determines this, set default false
	}

	var schedules []models.DevotionalSchedule
	err := db.DB.Preload("Devotional").
		Where("year = ? AND category = ? AND day_of_year >= ? AND day_of_year <= ?", year, category, startDay, endDay).
		Order("day_of_year asc").
		Find(&schedules).Error

	if err != nil {
		utils.SendError(c, http.StatusInternalServerError, "Database query failed", err.Error())
		return
	}

	items := make([]CalendarItem, 0)
	for _, s := range schedules {
		// Calculate actual date string (YYYY-MM-DD)
		d := time.Date(year, 1, 1, 0, 0, 0, 0, time.UTC).AddDate(0, 0, s.DayOfYear-1)
		items = append(items, CalendarItem{
			DayOfYear: s.DayOfYear,
			Date:      d.Format("2006-01-02"),
			Title:     s.Devotional.Title,
			Done:      false,
		})
	}

	utils.SendSuccess(c, http.StatusOK, "Calendar devotionals list retrieved", items)
}

// SubmitFeedbackHandler inserts a member's feedback from the mobile client.
func SubmitFeedbackHandler(c *gin.Context) {
	var req FeedbackRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.SendError(c, http.StatusBadRequest, "Invalid feedback input formats", err.Error())
		return
	}

	feedback := models.Feedback{
		Name:    req.Name,
		Email:   req.Email,
		Message: req.Message,
		IsRead:  false,
	}

	err := db.DB.Create(&feedback).Error
	if err != nil {
		utils.SendError(c, http.StatusInternalServerError, "Failed to submit feedback", err.Error())
		return
	}

	utils.SendSuccess(c, http.StatusCreated, "Feedback submitted successfully. Thank you!", feedback)
}

type DevotionalReadRequest struct {
	DevotionalID string `json:"devotional_id" binding:"required"`
}

// RecordDevotionalReadHandler stores a read click log event from the mobile app.
func RecordDevotionalReadHandler(c *gin.Context) {
	var req DevotionalReadRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.SendError(c, http.StatusBadRequest, "Invalid inputs", err.Error())
		return
	}

	devoID, err := uuid.Parse(req.DevotionalID)
	if err != nil {
		utils.SendError(c, http.StatusBadRequest, "Invalid devotional ID format", err.Error())
		return
	}

	readLog := models.DevotionalRead{
		DevotionalID: devoID,
		ReadAt:       time.Now(),
	}

	if err := db.DB.Create(&readLog).Error; err != nil {
		utils.SendError(c, http.StatusInternalServerError, "Failed to log devotional read", err.Error())
		return
	}

	utils.SendSuccess(c, http.StatusCreated, "Read logged successfully", nil)
}
