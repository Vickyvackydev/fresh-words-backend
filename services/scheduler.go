package services

import (
	"errors"
	"fmt"
	"math/rand"

	"fresh-words-backend/db"
	"fresh-words-backend/models"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// GenerateSchedule builds the DevotionalSchedule entries for a specific category and year.
func GenerateSchedule(category string, year int, packageID uuid.UUID) error {
	// 1. Fetch all devotionals from the given package/category
	var devotionals []models.Devotional
	err := db.DB.Where("package_id = ? AND category = ?", packageID, category).Order("default_day asc").Find(&devotionals).Error
	if err != nil {
		return fmt.Errorf("failed to fetch devotionals: %w", err)
	}

	if len(devotionals) == 0 {
		return errors.New("no devotionals found for the given package and category")
	}

	// 2. Determine number of days in the target year (leap year check)
	daysInYear := 365
	if isLeapYear(year) {
		daysInYear = 366
	}

	// 3. Clear existing schedule entries for this category and year to prevent duplicates
	err = db.DB.Where("year = ? AND category = ?", year, category).Delete(&models.DevotionalSchedule{}).Error
	if err != nil {
		return fmt.Errorf("failed to clear existing schedule: %w", err)
	}

	// 4. Generate mappings
	var schedules []models.DevotionalSchedule

	// Fetch active system settings for randomization flags
	var settings models.Settings
	if err := db.DB.First(&settings).Error; err != nil {
		// Fall back to defaults if not found
		settings = models.Settings{
			DailyDeliveranceRandomize: true,
			HolinessRandomize:         true,
			PrayerRandomize:           true,
		}
	}

	shouldRandomize := true
	if category == "Yearly Devotional" || category == "yearly" {
		shouldRandomize = false
	} else {
		switch category {
		case "Daily Deliverance":
			shouldRandomize = settings.DailyDeliveranceRandomize
		case "Holiness":
			shouldRandomize = settings.HolinessRandomize
		case "Prayer":
			shouldRandomize = settings.PrayerRandomize
		}
	}

	if !shouldRandomize {
		// Map strictly by default day order (no shuffling)
		for i := 1; i <= daysInYear; i++ {
			idx := (i - 1) % len(devotionals)
			dev := devotionals[idx]

			schedules = append(schedules, models.DevotionalSchedule{
				Year:         year,
				Category:     category,
				DayOfYear:    i,
				DevotionalID: dev.ID,
			})
		}
	} else {
		// Shuffled segments: Randomize sequence using year-seeded source
		r := rand.New(rand.NewSource(int64(year)))

		// Create a copy of devotionals array to shuffle
		shuffled := make([]models.Devotional, len(devotionals))
		copy(shuffled, devotionals)

		// Fisher-Yates Shuffle
		r.Shuffle(len(shuffled), func(i, j int) {
			shuffled[i], shuffled[j] = shuffled[j], shuffled[i]
		})

		for i := 1; i <= daysInYear; i++ {
			idx := (i - 1) % len(shuffled)
			dev := shuffled[idx]

			schedules = append(schedules, models.DevotionalSchedule{
				Year:         year,
				Category:     category,
				DayOfYear:    i,
				DevotionalID: dev.ID,
			})
		}
	}

	// 5. Batch insert schedules inside a database transaction
	return db.DB.Transaction(func(tx *gorm.DB) error {
		if len(schedules) > 0 {
			if err := tx.Create(&schedules).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

// isLeapYear returns true if year is a leap year.
func isLeapYear(year int) bool {
	return year%4 == 0 && (year%100 != 0 || year%400 == 0)
}
