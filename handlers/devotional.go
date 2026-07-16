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

type UpdateDevotionalRequest struct {
	Title              string `json:"title" binding:"required"`
	ScriptureReference string `json:"scripture_reference"`
	ScriptureQuote     string `json:"scripture_quote"`
	Body               string `json:"body" binding:"required"`
	Prayer             string `json:"prayer"`
	Reflection         string `json:"reflection"`
}

// UpdateDevotionalHandler updates the fields of a single devotional entry in the database.
func UpdateDevotionalHandler(c *gin.Context) {
	idStr := c.Param("id")
	devotionalID, err := uuid.Parse(idStr)
	if err != nil {
		utils.SendError(c, http.StatusBadRequest, "Invalid devotional ID format", err.Error())
		return
	}

	var req UpdateDevotionalRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.SendError(c, http.StatusBadRequest, "Invalid request body", err.Error())
		return
	}

	var devo models.Devotional
	err = db.DB.First(&devo, "id = ?", devotionalID).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			utils.SendError(c, http.StatusNotFound, "Devotional not found", nil)
			return
		}
		utils.SendError(c, http.StatusInternalServerError, "Database query failed", err.Error())
		return
	}

	// Update fields
	devo.Title = req.Title
	devo.ScriptureReference = req.ScriptureReference
	devo.ScriptureQuote = req.ScriptureQuote
	devo.Body = req.Body
	devo.Prayer = req.Prayer
	devo.Reflection = req.Reflection

	if err := db.DB.Save(&devo).Error; err != nil {
		utils.SendError(c, http.StatusInternalServerError, "Failed to update devotional", err.Error())
		return
	}

	utils.SendSuccess(c, http.StatusOK, "Devotional updated successfully", devo)
}
