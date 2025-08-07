package database

import (
	"log"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"

	models "authservice/pkg/models"
)

type DBConnection struct {
	*gorm.DB
}

func GetDBConnection() *DBConnection {
	connectionString := "u663533901_admin:Notkaruna@007@tcp(srv1619.hstgr.io:3306)/u663533901_auth_service?charset=utf8mb4&parseTime=True&loc=Local"

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