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

// UploadPackageHandler accepts a PDF/DOCX document, parses it, and creates or appends to a draft package.
func UploadPackageHandler(c *gin.Context) {
	category := c.PostForm("category")
	yearStr := c.PostForm("year")
	packageIDStr := c.PostForm("package_id")

	var packageID uuid.UUID
	var isAppend bool
	var existingPkg models.Package
	var year int

	if packageIDStr != "" {
		var err error
		packageID, err = uuid.Parse(packageIDStr)
		if err != nil {
			utils.SendError(c, http.StatusBadRequest, "Invalid package_id format", err.Error())
			return
		}
		
		err = db.DB.First(&existingPkg, "id = ?", packageID).Error
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				utils.SendError(c, http.StatusNotFound, "Package not found", nil)
				return
			}
			utils.SendError(c, http.StatusInternalServerError, "Database query failed", err.Error())
			return
		}

		if existingPkg.Status != "draft" {
			utils.SendError(c, http.StatusBadRequest, "Can only append to packages in draft status", nil)
			return
		}
		isAppend = true
		category = existingPkg.Category
		year = existingPkg.Year
	} else {
		if category == "" || yearStr == "" {
			utils.SendError(c, http.StatusBadRequest, "Category and year form values are required", nil)
			return
		}

		var err error
		year, err = strconv.Atoi(yearStr)
		if err != nil || year < 2000 {
			utils.SendError(c, http.StatusBadRequest, "Invalid year format", nil)
			return
		}
		packageID = uuid.New()
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
		utils.SendError(c, http.StatusUnprocessableEntity, "Document parsing failed — could not extract text from file", err.Error())
		return
	}

	// If no devotionals were extracted at all, bail early without saving
	if len(report.Devotionals) == 0 {
		utils.SendError(c, http.StatusUnprocessableEntity, "No devotional entries could be parsed from this document. Please check the document format and ensure each day starts with 'Month Day' (e.g. 'January 1') on its own line.", report.Issues)
		return
	}

	// Save Package and Devotionals in transaction (even if not fully valid — save as draft for review)
	dbErr := db.DB.Transaction(func(tx *gorm.DB) error {
		uploadedAt := time.Now()
		if len(report.Devotionals) > 0 {
			uploadedAt = report.Devotionals[0].CreatedAt
		}

		if isAppend {
			// Delete overlapping days
			var newDays []int
			for _, d := range report.Devotionals {
				newDays = append(newDays, d.DefaultDay)
			}
			if len(newDays) > 0 {
				if err := tx.Where("package_id = ? AND default_day IN ?", packageID, newDays).
					Delete(&models.Devotional{}).Error; err != nil {
					return err
				}
			}

			existingPkg.FileName = fmt.Sprintf("%s + %s", existingPkg.FileName, fileHeader.Filename)
			existingPkg.UploadedAt = uploadedAt
			if err := tx.Save(&existingPkg).Error; err != nil {
				return err
			}
		} else {
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

	var totalParsed int64 = int64(report.TotalParsed)
	if isAppend {
		db.DB.Model(&models.Devotional{}).Where("package_id = ?", packageID).Count(&totalParsed)
	}

	response := gin.H{
		"package_id":   packageID.String(),
		"total_parsed": totalParsed,
		"is_valid":     report.IsValid,
		"issues":       report.Issues,
	}

	message := "File uploaded and parsed successfully"
	if !report.IsValid || len(report.Issues) > 0 {
		message = fmt.Sprintf("File uploaded and saved as draft with %d issue(s). Review the issues before publishing.", len(report.Issues))
	}

	utils.SendSuccess(c, http.StatusOK, message, response)
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

// DeletePackageHandler deletes a package and all associated devotionals, schedules, bookmarks, and reads.
func DeletePackageHandler(c *gin.Context) {
	packageIDStr := c.Param("id")
	packageID, err := uuid.Parse(packageIDStr)
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

	// Transaction to delete package and all dependent objects to preserve database integrity
	err = db.DB.Transaction(func(tx *gorm.DB) error {
		// 1. Get all devotional IDs belonging to the package
		var devoIDs []uuid.UUID
		err := tx.Model(&models.Devotional{}).
			Where("package_id = ?", packageID).
			Pluck("id", &devoIDs).Error
		if err != nil {
			return err
		}

		if len(devoIDs) > 0 {
			// 2. Delete devotional schedules referencing these devotionals
			err = tx.Where("devotional_id IN ?", devoIDs).Delete(&models.DevotionalSchedule{}).Error
			if err != nil {
				return err
			}

			// 3. Delete user bookmarks referencing these devotionals
			err = tx.Where("devotional_id IN ?", devoIDs).Delete(&models.UserBookmark{}).Error
			if err != nil {
				return err
			}

			// 4. Delete read history referencing these devotionals
			err = tx.Where("devotional_id IN ?", devoIDs).Delete(&models.DevotionalRead{}).Error
			if err != nil {
				return err
			}

			// 5. Unscoped (hard) delete devotionals
			err = tx.Unscoped().Where("package_id = ?", packageID).Delete(&models.Devotional{}).Error
			if err != nil {
				return err
			}
		}

		// 6. Unscoped (hard) delete the package
		err = tx.Unscoped().Delete(&pkg).Error
		if err != nil {
			return err
		}

		return nil
	})

	if err != nil {
		utils.SendError(c, http.StatusInternalServerError, "Failed to delete package and its dependencies", err.Error())
		return
	}

	utils.SendSuccess(c, http.StatusOK, "Package and all associated data deleted successfully", nil)
}

