package handlers

import (
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"fresh-words-backend/db"
	"fresh-words-backend/models"
	"fresh-words-backend/services"
	"fresh-words-backend/utils"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type PublishRequest struct {
	PackageID string `json:"package_id" binding:"required"`
}

type RollbackRequest struct {
	PackageID string `json:"package_id" binding:"required"`
}

// UploadPackageHandler accepts a PDF/DOCX document, parses it, and creates a draft package.
func UploadPackageHandler(c *gin.Context) {
	category := c.PostForm("category")
	yearStr := c.PostForm("year")

	if category == "" || yearStr == "" {
		utils.SendError(c, http.StatusBadRequest, "Category and year form values are required", nil)
		return
	}

	year, err := strconv.Atoi(yearStr)
	if err != nil || year < 2000 {
		utils.SendError(c, http.StatusBadRequest, "Invalid year format", nil)
		return
	}

	fileHeader, err := c.FormFile("file")
	if err != nil {
		utils.SendError(c, http.StatusBadRequest, "File form field is required", err.Error())
		return
	}

	// Create uploads directory if it does not exist
	uploadsDir := "uploads"
	if err := os.MkdirAll(uploadsDir, 0755); err != nil {
		utils.SendError(c, http.StatusInternalServerError, "Failed to create uploads directory", err.Error())
		return
	}

	packageID := uuid.New()
	fileName := fmt.Sprintf("%s_%s", packageID.String(), fileHeader.Filename)
	filePath := filepath.Join(uploadsDir, fileName)

	if err := c.SaveUploadedFile(fileHeader, filePath); err != nil {
		utils.SendError(c, http.StatusInternalServerError, "Failed to save uploaded file", err.Error())
		return
	}
	defer os.Remove(filePath) // Cleanup file after parsing

	// Parse file
	report, err := services.ParseDocument(filePath, category, year, packageID)
	if err != nil {
		utils.SendError(c, http.StatusUnprocessableEntity, "Document parsing failed", err.Error())
		return
	}

	// Save Package and Devotionals in transaction
	dbErr := db.DB.Transaction(func(tx *gorm.DB) error {
		uploadedAt := time.Now()
		if len(report.Devotionals) > 0 {
			uploadedAt = report.Devotionals[0].CreatedAt
		}

		pkg := models.Package{
			ID:         packageID,
			Category:   category,
			Year:       year,
			Status:     "draft",
			FileName:   fileHeader.Filename,
			UploadedAt: uploadedAt,
		}

		if err := tx.Create(&pkg).Error; err != nil {
			return err
		}

		if len(report.Devotionals) > 0 {
			if err := tx.Create(&report.Devotionals).Error; err != nil {
				return err
			}
		}
		return nil
	})

	if dbErr != nil {
		utils.SendError(c, http.StatusInternalServerError, "Failed to save draft package to database", dbErr.Error())
		return
	}

	response := gin.H{
		"package_id":   packageID.String(),
		"total_parsed": report.TotalParsed,
		"is_valid":     report.IsValid,
		"issues":       report.Issues,
	}

	utils.SendSuccess(c, http.StatusOK, "File uploaded and parsed successfully", response)
}

// PublishPackageHandler transitions a draft package to published and generates the year schedules.
func PublishPackageHandler(c *gin.Context) {
	var req PublishRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.SendError(c, http.StatusBadRequest, "Invalid request format", err.Error())
		return
	}

	packageID, err := uuid.Parse(req.PackageID)
	if err != nil {
		utils.SendError(c, http.StatusBadRequest, "Invalid package ID format", err.Error())
		return
	}

	var pkg models.Package
	err = db.DB.First(&pkg, "id = ?", packageID).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			utils.SendError(c, http.StatusNotFound, "Package not found", nil)
			return
		}
		utils.SendError(c, http.StatusInternalServerError, "Database query failed", err.Error())
		return
	}

	if pkg.Status != "draft" {
		utils.SendError(c, http.StatusBadRequest, "Only packages in draft status can be published", nil)
		return
	}

	// Transaction to update package statuses and generate schedules
	err = db.DB.Transaction(func(tx *gorm.DB) error {
		// 1. Archive previous published packages in this category and year
		err := tx.Model(&models.Package{}).
			Where("category = ? AND year = ? AND status = ?", pkg.Category, pkg.Year, "published").
			Update("status", "archived").Error
		if err != nil {
			return err
		}

		// 2. Publish current package
		pkg.Status = "published"
		if err := tx.Save(&pkg).Error; err != nil {
			return err
		}

		return nil
	})

	if err != nil {
		utils.SendError(c, http.StatusInternalServerError, "Failed to publish package updates", err.Error())
		return
	}

	// 3. Generate scheduled calendar mapping for the year
	err = services.GenerateSchedule(pkg.Category, pkg.Year, pkg.ID)
	if err != nil {
		// Roll back status if schedule fails (manual safety fallback)
		db.DB.Model(&pkg).Update("status", "draft")
		utils.SendError(c, http.StatusInternalServerError, "Failed to build calendar schedule for the year", err.Error())
		return
	}

	utils.SendSuccess(c, http.StatusOK, "Package published and calendar schedule built successfully", pkg)
}

// RollbackPackageHandler restores an archived package as the published one.
func RollbackPackageHandler(c *gin.Context) {
	var req RollbackRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.SendError(c, http.StatusBadRequest, "Invalid request format", err.Error())
		return
	}

	packageID, err := uuid.Parse(req.PackageID)
	if err != nil {
		utils.SendError(c, http.StatusBadRequest, "Invalid package ID format", err.Error())
		return
	}

	var pkg models.Package
	err = db.DB.First(&pkg, "id = ?", packageID).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			utils.SendError(c, http.StatusNotFound, "Package not found", nil)
			return
		}
		utils.SendError(c, http.StatusInternalServerError, "Database query failed", err.Error())
		return
	}

	if pkg.Status != "archived" {
		utils.SendError(c, http.StatusBadRequest, "Only archived packages can be rolled back", nil)
		return
	}

	// Transaction to update statuses
	err = db.DB.Transaction(func(tx *gorm.DB) error {
		// Archive previous published packages
		err := tx.Model(&models.Package{}).
			Where("category = ? AND year = ? AND status = ?", pkg.Category, pkg.Year, "published").
			Update("status", "archived").Error
		if err != nil {
			return err
		}

		// Publish current package
		pkg.Status = "published"
		if err := tx.Save(&pkg).Error; err != nil {
			return err
		}

		return nil
	})

	if err != nil {
		utils.SendError(c, http.StatusInternalServerError, "Failed to update package statuses", err.Error())
		return
	}

	// Rebuild calendar schedule
	err = services.GenerateSchedule(pkg.Category, pkg.Year, pkg.ID)
	if err != nil {
		utils.SendError(c, http.StatusInternalServerError, "Failed to build calendar schedule for rolled back package", err.Error())
		return
	}

	utils.SendSuccess(c, http.StatusOK, "Rollback successful. Package is now active.", pkg)
}

// GetPackageHistoryHandler retrieves all uploaded packages in a specific category.
func GetPackageHistoryHandler(c *gin.Context) {
	category := c.Query("category")
	if category == "" {
		utils.SendError(c, http.StatusBadRequest, "Category parameter is required", nil)
		return
	}

	var packages []models.Package
	err := db.DB.Preload("Devotionals").Where("category = ?", category).Order("uploaded_at desc").Find(&packages).Error
	if err != nil {
		utils.SendError(c, http.StatusInternalServerError, "Failed to query package history", err.Error())
		return
	}

	utils.SendSuccess(c, http.StatusOK, "Package history list retrieved successfully", packages)
}
