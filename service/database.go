package main

import (
	"time"

	"github.com/jinzhu/gorm"
	_ "github.com/jinzhu/gorm/dialects/postgres"
)

// PostgreSQLStore for a PostgreSQL database.
type PostgreSQLStore struct {
	url string
}

func (s *PostgreSQLStore) initialize() error {
	db, err := s.getConnection()
	if err != nil {
		return err
	}
	defer db.Close()

	db.AutoMigrate(&UploadLink{})

	return nil
}

func (s *PostgreSQLStore) addUploadID(id string, expiry time.Time) error {
	db, err := s.getConnection()
	if err != nil {
		return err
	}
	defer db.Close()

	link := UploadLink{
		ID:     id,
		Expiry: expiry,
	}

	db.Create(&link)

	return nil
}

func (s *PostgreSQLStore) getUploadExpiry(id string) (*time.Time, error) {
	db, err := s.getConnection()
	if err != nil {
		return nil, err
	}
	defer db.Close()

	var link UploadLink
	db.Where("id = ?", id).First(&link)

	return &link.Expiry, nil
}

func (s *PostgreSQLStore) getConnection() (*gorm.DB, error) {
	return gorm.Open("postgres", s.url)
}
