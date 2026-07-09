package handlers

import (
	"fmt"
	"net/http"
	"strings"

	"fresh-words-backend/config"
	"fresh-words-backend/utils"
	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
)

// AuthMiddleware guards API routes, checking for valid JWT authorization.
func AuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			utils.SendError(c, http.StatusUnauthorized, "Missing Authorization Header", nil)
			c.Abort()
			return
		}

		parts := strings.Split(authHeader, " ")
		if len(parts) != 2 || strings.ToLower(parts[0]) != "bearer" {
			utils.SendError(c, http.StatusUnauthorized, "Invalid Authorization Header format (Bearer token required)", nil)
			c.Abort()
			return
		}

		tokenString := parts[1]
		token, err := jwt.Parse(tokenString, func(t *jwt.Token) (interface{}, error) {
			if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
			}
			return []byte(config.AppConfig.JWTSecret), nil
		})

		if err != nil || !token.Valid {
			utils.SendError(c, http.StatusUnauthorized, "Invalid or expired token", nil)
			c.Abort()
			return
		}

		claims, ok := token.Claims.(jwt.MapClaims)
		if !ok {
			utils.SendError(c, http.StatusUnauthorized, "Invalid token claims", nil)
			c.Abort()
			return
		}

		userID, ok := claims["sub"].(string)
		if !ok {
			utils.SendError(c, http.StatusUnauthorized, "Invalid token format", nil)
			c.Abort()
			return
		}

		c.Set("userID", userID)
		c.Next()
	}
}
