package database

import (
	"log"
	"os"

	"github.com/joho/godotenv"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"

	models "authservice/pkg/models"
)

type DBConnection struct {
	*gorm.DB
}

func init() {
	if err := godotenv.Load(); err != nil {
		log.Println()
	}
}

func GetDBConnection() *DBConnection {

	connectionString := os.Getenv("DB_CONNECTION_STRING")

	db, err := gorm.Open(mysql.Open(connectionString), &gorm.Config{})
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}

	return &DBConnection{
		db,
	}
}

func (dbCon *DBConnection) CreateTables() {
	allModels := models.GetAllModels()
	err := dbCon.AutoMigrate(allModels...)
	if err != nil {
		log.Fatal("Failed to create tables in database")
	}
	log.Println("All tables created")
}