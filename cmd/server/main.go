package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"fresh-words-backend/config"
	"fresh-words-backend/db"
	"fresh-words-backend/handlers"
	"github.com/gin-gonic/gin"
)

func main() {
	// 1. Initialize configuration settings
	config.LoadConfig()

	// Set Gin mode
	if config.AppConfig.Env == "production" {
		gin.SetMode(gin.ReleaseMode)
	}

	// 2. Establish PostgreSQL database connection pool
	db.ConnectDB()

	// 3. Bootstrap default credentials
	handlers.BootstrapAdmin()

	// 4. Initialize router
	router := gin.Default()

	// Bind CORS middleware
	router.Use(corsMiddleware())

	// Set up upload size limit (e.g. 50MB)
	router.MaxMultipartMemory = 50 << 20

	// 5. Register Routes
	registerRoutes(router)

	// 6. Graceful Shutdown HTTP Server Setup
	serverAddr := fmt.Sprintf(":%s", config.AppConfig.Port)
	srv := &http.Server{
		Addr:    serverAddr,
		Handler: router,
	}

	// Run server in a goroutine
	go func() {
		log.Printf("Fresh Words Backend listening on %s in %s mode\n", serverAddr, config.AppConfig.Env)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("listen error: %s\n", err)
		}
	}()

	// Wait for interrupt signal to gracefully shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Println("Shutting down backend server gracefully...")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		log.Fatal("Server forced to shutdown:", err)
	}

	log.Println("Server exiting")
}

func corsMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Writer.Header().Set("Access-Control-Allow-Origin", "*")
		c.Writer.Header().Set("Access-Control-Allow-Credentials", "true")
		c.Writer.Header().Set("Access-Control-Allow-Headers", "Content-Type, Content-Length, Accept-Encoding, X-CSRF-Token, Authorization, accept, origin, Cache-Control, X-Requested-With")
		c.Writer.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS, GET, PUT, DELETE")

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}

		c.Next()
	}
}

func registerRoutes(r *gin.Engine) {
	// Base API check
	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "healthy", "time": time.Now().Format(time.RFC3339)})
	})

	api := r.Group("/api/v1")
	{
		// Public Auth
		api.POST("/auth/login", handlers.LoginHandler)

		// Public Client Devotional Access (Mobile app endpoints)
		api.GET("/devotionals/today", handlers.GetTodayDevotionalHandler)
		api.GET("/devotionals/date", handlers.GetDevotionalByDateHandler)
		api.GET("/devotionals/calendar", handlers.GetCalendarDevotionalsHandler)
		api.GET("/packages/active", handlers.GetActivePackageDevotionalsHandler)
		api.GET("/settings", handlers.GetSettingsHandler)
		api.POST("/feedback", handlers.SubmitFeedbackHandler)
		api.GET("/bookmarks", handlers.GetBookmarksHandler)
		api.POST("/bookmarks", handlers.ToggleBookmarkHandler)
		api.POST("/devotionals/read", handlers.RecordDevotionalReadHandler)

		// Protected Admin routes
		admin := api.Group("/admin")
		admin.Use(handlers.AuthMiddleware())
		{
			// Dashboard Statistics
			admin.GET("/dashboard", handlers.GetDashboardStatsHandler)

			// Congregation Feedback Management
			admin.GET("/feedback", handlers.GetFeedbackHandler)
			admin.PUT("/feedback/:id/read", handlers.MarkFeedbackReadHandler)
			admin.DELETE("/feedback/:id", handlers.DeleteFeedbackHandler)

			// General Brand Metadata Settings
			admin.GET("/settings", handlers.GetSettingsHandler)
			admin.PUT("/settings", handlers.UpdateSettingsHandler)

			// Devotional Package Operations
			admin.POST("/packages/upload", handlers.UploadPackageHandler)
			admin.POST("/packages/publish", handlers.PublishPackageHandler)
			admin.POST("/packages/rollback", handlers.RollbackPackageHandler)
			admin.GET("/packages/history", handlers.GetPackageHistoryHandler)
			admin.DELETE("/packages/:id", handlers.DeletePackageHandler)

			// Devotional Operations
			admin.PUT("/devotionals/:id", handlers.UpdateDevotionalHandler)
		}
	}
}
