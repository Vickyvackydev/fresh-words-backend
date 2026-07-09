package db

import (
	"fmt"
	"log"
	"os"
	"time"

	"fresh-words-backend/config"
	"fresh-words-backend/models"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

var DB *gorm.DB

func ConnectDB() {
	cfg := config.AppConfig

	// 1. Establish temporary connection to default 'postgres' database to ensure target database exists
	tempDsn := fmt.Sprintf(
		"host=%s user=%s password=%s dbname=postgres port=%s sslmode=%s TimeZone=UTC",
		cfg.DBHost, cfg.DBUser, cfg.DBPassword, cfg.DBPort, cfg.DBSSLMode,
	)
	tempDb, tempErr := gorm.Open(postgres.Open(tempDsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if tempErr == nil {
		var count int
		tempDb.Raw("SELECT COUNT(*) FROM pg_database WHERE datname = ?", cfg.DBName).Scan(&count)
		if count == 0 {
			log.Printf("Database %q does not exist, creating it...\n", cfg.DBName)
			createSql := fmt.Sprintf("CREATE DATABASE %s", cfg.DBName)
			if err := tempDb.Exec(createSql).Error; err != nil {
				log.Printf("Warning: Failed to create database %q dynamically: %v\n", cfg.DBName, err)
			} else {
				log.Printf("Database %q successfully created!\n", cfg.DBName)
			}
		}
		sqlDb, _ := tempDb.DB()
		if sqlDb != nil {
			sqlDb.Close()
		}
	} else {
		log.Printf("Warning: Could not connect to default 'postgres' database: %v\n", tempErr)
	}

	dsn := fmt.Sprintf(
		"host=%s user=%s password=%s dbname=%s port=%s sslmode=%s TimeZone=UTC",
		cfg.DBHost, cfg.DBUser, cfg.DBPassword, cfg.DBName, cfg.DBPort, cfg.DBSSLMode,
	)

	var err error
	gormConfig := &gorm.Config{}

	if cfg.Env == "production" {
		gormConfig.Logger = logger.Default.LogMode(logger.Error)
	} else {
		gormConfig.Logger = logger.Default.LogMode(logger.Info)
	}

	for i := 1; i <= 5; i++ {
		DB, err = gorm.Open(postgres.Open(dsn), gormConfig)
		if err == nil {
			break
		}
		log.Printf("Failed to connect to database (attempt %d/5): %v. Retrying in 3 seconds...", i, err)
		time.Sleep(3 * time.Second)
	}

	if err != nil {
		log.Fatalf("Could not connect to the database: %v", err)
	}

	log.Println("Database connection successfully established!")

	// Enable pgcrypto extension to support gen_random_uuid()
	if err := DB.Exec("CREATE EXTENSION IF NOT EXISTS \"pgcrypto\"").Error; err != nil {
		log.Printf("Warning: Failed to enable pgcrypto extension: %v\n", err)
	}

	// Run GORM AutoMigrate
	if os.Getenv("SKIP_MIGRATION") != "true" {
		err = DB.AutoMigrate(
			&models.User{},
			&models.Package{},
			&models.Feedback{},
			&models.Settings{},
			&models.Devotional{},
			&models.DevotionalSchedule{},
			&models.UserBookmark{},
			&models.DevotionalRead{},
		)
		if err != nil {
			log.Fatalf("Database AutoMigrate failed: %v", err)
		}
		log.Println("Database AutoMigrate completed successfully!")
	} else {
		log.Println("Database AutoMigrate skipped because SKIP_MIGRATION is set to true")
	}

	sqlDB, err := DB.DB()
	if err != nil {
		log.Fatalf("Could not obtain sql.DB pool reference: %v", err)
	}

	sqlDB.SetMaxIdleConns(10)
	sqlDB.SetMaxOpenConns(100)
	sqlDB.SetConnMaxLifetime(time.Hour)
}
