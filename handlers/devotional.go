package handlers

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"fresh-words-backend/db"
	"fresh-words-backend/models"
	"fresh-words-backend/services"
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

// AdminGetDevotionalTranslationHandler retrieves the verified translation for a devotional
func AdminGetDevotionalTranslationHandler(c *gin.Context) {
	idStr := strings.TrimSpace(c.Param("id"))
	lang := strings.ToLower(strings.TrimSpace(c.DefaultQuery("lang", "yo")))

	devotionalID, err := uuid.Parse(idStr)
	if err != nil {
		utils.SendError(c, http.StatusBadRequest, "Invalid devotional ID format", err.Error())
		return
	}

	var devo models.Devotional
	if err := db.DB.First(&devo, "id = ?", devotionalID).Error; err != nil {
		utils.SendError(c, http.StatusNotFound, "Devotional not found", nil)
		return
	}

	var trans models.ContentTranslation
	findErr := db.DB.Where("content_type = ? AND content_id = ? AND language = ?", "devotional", devotionalID.String(), lang).First(&trans).Error
	if findErr != nil {
		utils.SendSuccess(c, http.StatusOK, "Translation not found, returning empty template", gin.H{
			"content_id":          devotionalID.String(),
			"language":            lang,
			"title":               "",
			"scripture_quote":     "",
			"scripture_reference": "",
			"body":                "",
			"prayer":              "",
			"reflection":          "",
		})
		return
	}

	var payload services.DevotionalTranslationPayload
	if err := json.Unmarshal([]byte(trans.TranslatedData), &payload); err != nil {
		utils.SendError(c, http.StatusInternalServerError, "Failed to parse translation data", err.Error())
		return
	}

	utils.SendSuccess(c, http.StatusOK, "Devotional translation retrieved successfully", gin.H{
		"content_id":          devotionalID.String(),
		"language":            lang,
		"title":               payload.Title,
		"scripture_quote":     payload.ScriptureQuote,
		"scripture_reference": payload.ScriptureReference,
		"body":                payload.Body,
		"prayer":              payload.Prayer,
		"reflection":          payload.Reflection,
	})
}

// AdminSaveDevotionalTranslationHandler saves or updates the verified translation for a devotional
func AdminSaveDevotionalTranslationHandler(c *gin.Context) {
	idStr := strings.TrimSpace(c.Param("id"))
	lang := strings.ToLower(strings.TrimSpace(c.DefaultQuery("lang", "yo")))

	devotionalID, err := uuid.Parse(idStr)
	if err != nil {
		utils.SendError(c, http.StatusBadRequest, "Invalid devotional ID format", err.Error())
		return
	}

	var devo models.Devotional
	if err := db.DB.First(&devo, "id = ?", devotionalID).Error; err != nil {
		utils.SendError(c, http.StatusNotFound, "Devotional not found", nil)
		return
	}

	var req services.DevotionalTranslationPayload
	if bindErr := c.ShouldBindJSON(&req); bindErr != nil {
		utils.SendError(c, http.StatusBadRequest, "Invalid translation body", bindErr.Error())
		return
	}

	dataBytes, _ := json.Marshal(req)

	var existing models.ContentTranslation
	findErr := db.DB.Where("content_type = ? AND content_id = ? AND language = ?", "devotional", devotionalID.String(), lang).First(&existing).Error
	if findErr == nil {
		existing.TranslatedData = string(dataBytes)
		if saveErr := db.DB.Save(&existing).Error; saveErr != nil {
			utils.SendError(c, http.StatusInternalServerError, "Failed to update devotional translation", saveErr.Error())
			return
		}
	} else {
		newTrans := models.ContentTranslation{
			ContentType:    "devotional",
			ContentID:      devotionalID.String(),
			Language:       lang,
			TranslatedData: string(dataBytes),
		}
		if createErr := db.DB.Create(&newTrans).Error; createErr != nil {
			utils.SendError(c, http.StatusInternalServerError, "Failed to create devotional translation", createErr.Error())
			return
		}
	}

	utils.SendSuccess(c, http.StatusOK, "Devotional translation saved successfully", req)
}

// AdminGenerateDevotionalAIDraftHandler produces an AI translation draft for admin review
func AdminGenerateDevotionalAIDraftHandler(c *gin.Context) {
	idStr := strings.TrimSpace(c.Param("id"))
	lang := strings.ToLower(strings.TrimSpace(c.DefaultQuery("lang", "yo")))

	devotionalID, err := uuid.Parse(idStr)
	if err != nil {
		utils.SendError(c, http.StatusBadRequest, "Invalid devotional ID format", err.Error())
		return
	}

	var devo models.Devotional
	if err := db.DB.First(&devo, "id = ?", devotionalID).Error; err != nil {
		utils.SendError(c, http.StatusNotFound, "Devotional not found", nil)
		return
	}

	draft, draftErr := services.GenerateAIDevotionalDraft(&devo, lang)
	if draftErr != nil {
		utils.SendError(c, http.StatusInternalServerError, "Failed to generate AI draft: "+draftErr.Error(), nil)
		return
	}

	utils.SendSuccess(c, http.StatusOK, "AI translation draft generated successfully", draft)
}

type CreateDevotionalRequest struct {
	Category           string `json:"category" binding:"required"`
	PackageID          string `json:"package_id"`
	DefaultDay         int    `json:"default_day" binding:"required"`
	Title              string `json:"title" binding:"required"`
	ScriptureReference string `json:"scripture_reference" binding:"required"`
	ScriptureQuote     string `json:"scripture_quote" binding:"required"`
	Body               string `json:"body" binding:"required"`
	Prayer             string `json:"prayer"`
	Reflection         string `json:"reflection"`
	ActionPoints       string `json:"action_points"`
}

// AdminCreateDevotionalHandler creates a single new devotional entry attached to a category package
func AdminCreateDevotionalHandler(c *gin.Context) {
	var req CreateDevotionalRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.SendError(c, http.StatusBadRequest, "Invalid request body: "+err.Error(), err.Error())
		return
	}

	cleanCategory := strings.TrimSpace(req.Category)
	if cleanCategory == "" {
		cleanCategory = "Daily Deliverance"
	}

	var targetPkg models.Package
	if strings.TrimSpace(req.PackageID) != "" {
		if pkgUUID, err := uuid.Parse(req.PackageID); err == nil {
			_ = db.DB.First(&targetPkg, "id = ?", pkgUUID).Error
		}
	}

	if targetPkg.ID == uuid.Nil {
		// 1. Look for published package in this category
		err := db.DB.Where("category = ? AND status = ?", cleanCategory, "published").Order("year desc").First(&targetPkg).Error
		if err != nil {
			// 2. Look for any draft package in this category
			err = db.DB.Where("category = ?", cleanCategory).Order("created_at desc").First(&targetPkg).Error
			if err != nil {
				// 3. Auto-create a package for this category so devotionals have a valid container
				targetPkg = models.Package{
					ID:         uuid.New(),
					Category:   cleanCategory,
					Year:       time.Now().Year(),
					Status:     "published",
					FileName:   cleanCategory + "_manual_entries.json",
					UploadedAt: time.Now(),
				}
				if err := db.DB.Create(&targetPkg).Error; err != nil {
					utils.SendError(c, http.StatusInternalServerError, "Failed to create package container", err.Error())
					return
				}
			}
		}
	}

	// Format action points as valid JSON array if string provided
	actionPointsJSON := "[]"
	if strings.TrimSpace(req.ActionPoints) != "" {
		trimmed := strings.TrimSpace(req.ActionPoints)
		if strings.HasPrefix(trimmed, "[") {
			actionPointsJSON = trimmed
		} else {
			lines := strings.Split(trimmed, "\n")
			var clean []string
			for _, l := range lines {
				l = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(l), "-"))
				l = strings.TrimSpace(strings.TrimPrefix(l, "•"))
				if l != "" {
					clean = append(clean, l)
				}
			}
			b, _ := json.Marshal(clean)
			actionPointsJSON = string(b)
		}
	}

	newDevo := models.Devotional{
		ID:                 uuid.New(),
		PackageID:          targetPkg.ID,
		Category:           cleanCategory,
		DefaultDay:         req.DefaultDay,
		Title:              strings.TrimSpace(req.Title),
		ScriptureReference: strings.TrimSpace(req.ScriptureReference),
		ScriptureQuote:     strings.TrimSpace(req.ScriptureQuote),
		Body:               strings.TrimSpace(req.Body),
		Prayer:             strings.TrimSpace(req.Prayer),
		Reflection:         strings.TrimSpace(req.Reflection),
		ActionPoints:       actionPointsJSON,
	}

	if err := db.DB.Create(&newDevo).Error; err != nil {
		utils.SendError(c, http.StatusInternalServerError, "Failed to create devotional entry: "+err.Error(), err.Error())
		return
	}

	// Update package updated_at
	db.DB.Model(&models.Package{}).Where("id = ?", targetPkg.ID).Update("updated_at", time.Now())

	utils.SendSuccess(c, http.StatusCreated, "Devotional created successfully", newDevo)
}

// AdminDeleteDevotionalHandler deletes a single devotional by ID
func AdminDeleteDevotionalHandler(c *gin.Context) {
	idStr := strings.TrimSpace(c.Param("id"))
	devotionalID, err := uuid.Parse(idStr)
	if err != nil {
		utils.SendError(c, http.StatusBadRequest, "Invalid devotional ID format", err.Error())
		return
	}

	var devo models.Devotional
	if err := db.DB.First(&devo, "id = ?", devotionalID).Error; err != nil {
		utils.SendError(c, http.StatusNotFound, "Devotional not found", nil)
		return
	}

	packageID := devo.PackageID

	if err := db.DB.Delete(&devo).Error; err != nil {
		utils.SendError(c, http.StatusInternalServerError, "Failed to delete devotional", err.Error())
		return
	}

	// Update package total count
	var totalCount int64
	db.DB.Model(&models.Devotional{}).Where("package_id = ?", packageID).Count(&totalCount)
	db.DB.Model(&models.Package{}).Where("id = ?", packageID).Update("total_count", int(totalCount))

	utils.SendSuccess(c, http.StatusOK, "Devotional deleted successfully", nil)
}
