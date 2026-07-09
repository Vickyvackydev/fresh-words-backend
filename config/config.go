package config

import (
	"log"
	"os"

	"github.com/joho/godotenv"
)

type Config struct {
	Port          string
	Env           string
	DBHost        string
	DBPort        string
	DBUser        string
	DBPassword    string
	DBName        string
	DBSSLMode     string
	JWTSecret     string
	AdminEmail    string
	AdminPassword string
}

var AppConfig Config

func LoadConfig() {
	// Load .env file if it exists
	if err := godotenv.Load(); err != nil {
		log.Println("No .env file found, using system environment variables")
	}

	AppConfig = Config{
		Port:          getEnv("PORT", "8080"),
		Env:           getEnv("ENV", "development"),
		DBHost:        getEnv("DB_HOST", "127.0.0.1"),
		DBPort:        getEnv("DB_PORT", "5432"),
		DBUser:        os.Getenv("DB_USER"),
		DBPassword:    os.Getenv("DB_PASSWORD"),
		DBName:        os.Getenv("DB_NAME"),
		DBSSLMode:     getEnv("DB_SSLMODE", "disable"),
		JWTSecret:     os.Getenv("JWT_SECRET"),
		AdminEmail:    os.Getenv("ADMIN_EMAIL"),
		AdminPassword: os.Getenv("ADMIN_PASSWORD"),
	}

	// Validate critical variables
	if AppConfig.DBUser == "" || AppConfig.DBPassword == "" || AppConfig.DBName == "" || AppConfig.JWTSecret == "" {
		log.Println("WARNING: Some critical environment variables (DB_USER, DB_PASSWORD, DB_NAME, JWT_SECRET) are not set. Database connections or authentication may fail.")
	}
}

func getEnv(key, defaultValue string) string {
	if value, exists := os.LookupEnv(key); exists {
		return value
	}
	return defaultValue
}
