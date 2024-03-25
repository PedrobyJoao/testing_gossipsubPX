package db

import (
	"fmt"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/PedrobyJoao/koko/models"
)

var DB *gorm.DB

func New() error {
	database, err := gorm.Open(sqlite.Open("koko.db"), &gorm.Config{})
	if err != nil {
		return fmt.Errorf("Failed to connect to database!")
	}

	database.AutoMigrate(&models.HostInfo{})

	DB = database

	return nil
}
