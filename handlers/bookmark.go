package handlers

import (
	"errors"
	"net/http"

	"fresh-words-backend/db"
	"fresh-words-backend/models"
	"fresh-words-backend/utils"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type ToggleBookmarkRequest struct {
	DeviceID     string `json:"device_id" binding:"required"`
	DevotionalID string `json:"devotional_id" binding:"required"`
}

// ToggleBookmarkHandler adds a bookmark if it doesn't exist, or removes it if it does.
func ToggleBookmarkHandler(c *gin.Context) {
	var req ToggleBookmarkRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.SendError(c, http.StatusBadRequest, "Invalid inputs", err.Error())
		return
	}

	devoID, err := uuid.Parse(req.DevotionalID)
	if err != nil {
		utils.SendError(c, http.StatusBadRequest, "Invalid devotional ID format", err.Error())
		return
	}

	// 1. Verify devotional exists
	var devo models.Devotional
	if err := db.DB.First(&devo, "id = ?", devoID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			utils.SendError(c, http.StatusNotFound, "Devotional not found", nil)
			return
		}
		utils.SendError(c, http.StatusInternalServerError, "Database query failed", err.Error())
		return
	}

	// 2. Check if already bookmarked
	var bookmark models.UserBookmark
	err = db.DB.Where("device_id = ? AND devotional_id = ?", req.DeviceID, devoID).First(&bookmark).Error

	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			// Not bookmarked: Create bookmark
			newBookmark := models.UserBookmark{
				DeviceID:     req.DeviceID,
				DevotionalID: devoID,
			}
			if err := db.DB.Create(&newBookmark).Error; err != nil {
				utils.SendError(c, http.StatusInternalServerError, "Failed to create bookmark", err.Error())
				return
			}
			utils.SendSuccess(c, http.StatusCreated, "Bookmark added successfully", gin.H{"bookmarked": true})
			return
		}
		utils.SendError(c, http.StatusInternalServerError, "Database query failed", err.Error())
		return
	}

	// Already bookmarked: Delete bookmark
	if err := db.DB.Delete(&bookmark).Error; err != nil {
		utils.SendError(c, http.StatusInternalServerError, "Failed to remove bookmark", err.Error())
		return
	}

	utils.SendSuccess(c, http.StatusOK, "Bookmark removed successfully", gin.H{"bookmarked": false})
}

// GetBookmarksHandler retrieves all bookmarked devotionals for a given device.
func GetBookmarksHandler(c *gin.Context) {
	deviceID := c.Query("device_id")
	if deviceID == "" {
		utils.SendError(c, http.StatusBadRequest, "device_id query parameter is required", nil)
		return
	}

	var bookmarks []models.UserBookmark
	err := db.DB.Preload("Devotional").Where("device_id = ?", deviceID).Order("created_at desc").Find(&bookmarks).Error
	if err != nil {
		utils.SendError(c, http.StatusInternalServerError, "Failed to load bookmarks", err.Error())
		return
	}

	// Extract devotionals (filtering out any nil relation safety)
	var devotionals []models.Devotional
	for _, b := range bookmarks {
		if b.Devotional.ID != uuid.Nil {
			devotionals = append(devotionals, b.Devotional)
		}
	}

	utils.SendSuccess(c, http.StatusOK, "Bookmarks retrieved successfully", devotionals)
}