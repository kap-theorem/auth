package main

import (
"context"
"fmt"
"log"

"authservice/internal/database"
"authservice/pkg/models"
"authservice/pkg/utils"
"github.com/google/uuid"
)

func main() {
db := database.GetDBConnection()

hash, err := utils.HashPassword("demo")
if err != nil {
log.Fatalf("hash err: %v", err)
}

demoUser := models.User{
UserID:    uuid.New().String(),
UserName:  "demo",
Email:     "demo@kaplabs.dev",
Password:  hash,
ClientID:  "555798ba-c23b-4757-9cd3-8944de52df81",
}

result := db.WithContext(context.Background()).Create(&demoUser)
if result.Error != nil {
fmt.Printf("Create demo failed: %v\n", result.Error)
} else {
fmt.Printf("Create demo success\n")
}
}
