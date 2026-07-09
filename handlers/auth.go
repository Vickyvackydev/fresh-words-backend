package handlers

import (
	"errors"
	"log"
	"net/http"
	"time"

	"fresh-words-backend/config"
	"fresh-words-backend/db"
	"fresh-words-backend/models"
	"fresh-words-backend/utils"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

type LoginRequest struct {
	Email    string `json:"email" binding:"required,email"`
	Password string `json:"password" binding:"required"`
}

type AuthResponse struct {
	Token string      `json:"token"`
	User  models.User `json:"user"`
}

// BootstrapAdmin ensures at least one admin exists in the database.
func BootstrapAdmin() {
	email := config.AppConfig.AdminEmail

	password := config.AppConfig.AdminPassword
	

	var user models.User
	err := db.DB.Where("email = ?", email).First(&user).Error
	if err != nil && errors.Is(err, gorm.ErrRecordNotFound) {
		hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
		if err != nil {
			log.Printf("Failed to hash bootstrap admin password: %v", err)
			return
		}

		admin := models.User{
			Email:    email,
			Password: string(hashedPassword),
		}

		if err := db.DB.Create(&admin).Error; err != nil {
			log.Printf("Failed to bootstrap admin: %v", err)
		} else {
			log.Println("Admin bootstrapped successfully with email:", admin.Email)
		}
	} else if err == nil {
		log.Println("Admin user already exists:", user.Email)
	}
}

// LoginHandler processes admin login credentials.
func LoginHandler(c *gin.Context) {
	var req LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.SendError(c, http.StatusBadRequest, "Invalid input request formats", err.Error())
		return
	}

	var user models.User
	err := db.DB.Where("email = ?", req.Email).First(&user).Error
	if err != nil {
		utils.SendError(c, http.StatusUnauthorized, "Invalid credentials", nil)
		return
	}

	err = bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(req.Password))
	if err != nil {
		utils.SendError(c, http.StatusUnauthorized, "Invalid credentials", nil)
		return
	}

	// Generate JWT token
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub": user.ID.String(),
		"exp": time.Now().Add(time.Hour * 24 * 7).Unix(), // 7 days
		"iat": time.Now().Unix(),
	})

	tokenString, err := token.SignedString([]byte(config.AppConfig.JWTSecret))
	if err != nil {
		utils.SendError(c, http.StatusInternalServerError, "Could not generate auth token", err.Error())
		return
	}

	utils.SendSuccess(c, http.StatusOK, "Login successful", AuthResponse{
		Token: tokenString,
		User:  user,
	})
}
