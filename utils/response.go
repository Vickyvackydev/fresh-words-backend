package utils

import (
	"math"

	"github.com/gin-gonic/gin"
)

type APIResponse struct {
	Success bool        `json:"success"`
	Message string      `json:"message,omitempty"`
	Data    interface{} `json:"data,omitempty"`
	Errors  interface{} `json:"errors,omitempty"`
}

type PaginatedData struct {
	Items      interface{} `json:"items"`
	Total      int64       `json:"total"`
	Page       int         `json:"page"`
	Limit      int         `json:"limit"`
	TotalPages int         `json:"total_pages"`
}

// SendSuccess sends a unified JSON success response
func SendSuccess(c *gin.Context, status int, message string, data interface{}) {
	c.JSON(status, APIResponse{
		Success: true,
		Message: message,
		Data:    data,
	})
}

// SendError sends a unified JSON error response
func SendError(c *gin.Context, status int, message string, errors interface{}) {
	c.JSON(status, APIResponse{
		Success: false,
		Message: message,
		Errors:  errors,
	})
}

// SendPaginated sends a success response wrapped in standard pagination metadata
func SendPaginated(c *gin.Context, status int, message string, items interface{}, total int64, page int, limit int) {
	totalPages := int(math.Ceil(float64(total) / float64(limit)))
	if totalPages == 0 {
		totalPages = 1
	}
	c.JSON(status, APIResponse{
		Success: true,
		Message: message,
		Data: PaginatedData{
			Items:      items,
			Total:      total,
			Page:       page,
			Limit:      limit,
			TotalPages: totalPages,
		},
	})
}
