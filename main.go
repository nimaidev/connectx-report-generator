package main

import (
	"fmt"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/joho/godotenv"
	"github.com/sirupsen/logrus"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

type AppConfig struct {
	MySqlHost string
	MySqlPort int
	MySqlDb   string
	MySqlUser string
	MySqlPass string
	ServerUrl string
}

type ControllerMaster struct {
	Id             int16     `gorm:"primarykey" json:"id"`
	OrgId          int       `json:"orgId"`
	ControllerName string    `json:"controllerName"`
	MacAddress     string    `json:"macAddress"`
	Password       string    `json:"password"`
	Token          string    `json:"token"`
	LastHeartBeat  time.Time `json:"lastHeartbeat"`
}

var config AppConfig

func loadConfig() error {
	if err := godotenv.Load(); err != nil {
		return fmt.Errorf("unable to load .env file: %w", err)
	}

	port, err := strconv.Atoi(os.Getenv("MYSQL_PORT"))
	if err != nil {
		return fmt.Errorf("invalid MYSQL_PORT in .env file: %w", err)
	}

	config = AppConfig{
		MySqlHost: os.Getenv("MYSQL_HOST"),
		MySqlPort: port,
		MySqlDb:   os.Getenv("MYSQL_DB"),
		MySqlUser: os.Getenv("MYSQL_USER"),
		MySqlPass: os.Getenv("MYSQL_PASS"),
		ServerUrl: os.Getenv("SERVER_URL"),
	}

	log.WithFields(logrus.Fields{
		"host": config.MySqlHost,
		"port": config.MySqlPort,
		"db":   config.MySqlDb,
	}).Info("Configuration loaded successfully")

	return nil
}

func initDatabase() (*gorm.DB, error) {
	// Connect without database to create it if needed
	dsnWithoutDb := fmt.Sprintf("%s:%s@tcp(%s:%d)/?charset=utf8mb4&parseTime=True&loc=Local",
		config.MySqlUser, config.MySqlPass, config.MySqlHost, config.MySqlPort)

	log.Debug("Connecting to MySQL server")
	db, err := gorm.Open(mysql.Open(dsnWithoutDb), &gorm.Config{})
	if err != nil {
		return nil, fmt.Errorf("failed to connect to MySQL: %w", err)
	}

	// Create database if not exists
	log.WithField("database", config.MySqlDb).Info("Creating database if not exists")
	if err := db.Exec(fmt.Sprintf("CREATE DATABASE IF NOT EXISTS `%s`", config.MySqlDb)).Error; err != nil {
		return nil, fmt.Errorf("failed to create database: %w", err)
	}

	// Close the connection and reconnect with the database
	sqlDB, _ := db.DB()
	sqlDB.Close()

	// Connect to the specific database
	dsn := fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?charset=utf8mb4&parseTime=True&loc=Local",
		config.MySqlUser, config.MySqlPass, config.MySqlHost, config.MySqlPort, config.MySqlDb)

	log.WithField("database", config.MySqlDb).Info("Connecting to database")
	db, err = gorm.Open(mysql.Open(dsn), &gorm.Config{})
	if err != nil {
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}

	// Auto-migrate tables
	log.Info("Running auto-migration")
	if err := db.AutoMigrate(&ControllerMaster{}); err != nil {
		return nil, fmt.Errorf("failed to auto-migrate tables: %w", err)
	}

	log.Info("Database initialization completed successfully")
	return db, nil
}

func init() {
	initLogger()

	if err := loadConfig(); err != nil {
		log.WithError(err).Fatal("Failed to load configuration")
	}
}

func main() {
	log.Info("Starting connectx application")

	db, err := initDatabase()
	if err != nil {
		log.WithError(err).Fatal("Failed to initialize database")
	}

	go config.StartGateWayOperation(db)

	log.Info("Application started successfully")

	// Set up signal handling for graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	// Wait for interrupt signal
	<-sigChan
	log.Info("Shutdown signal received, exiting...")
}
