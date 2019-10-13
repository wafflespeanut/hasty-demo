package main

import (
	"strings"
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
	db.AutoMigrate(&ImageMeta{})

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

func (s *PostgreSQLStore) addImageMeta(meta ImageMeta) error {
	db, err := s.getConnection()
	if err != nil {
		return err
	}
	defer db.Close()

	db.Create(&meta)
	return nil
}

func (s *PostgreSQLStore) updateImageMeta(meta ImageMeta) error {
	db, err := s.getConnection()
	if err != nil {
		return err
	}
	defer db.Close()

	db.Save(&meta)
	return nil
}

func (s *PostgreSQLStore) fetchImageMeta(id string) (*ImageMeta, error) {
	db, err := s.getConnection()
	if err != nil {
		return nil, err
	}
	defer db.Close()

	var meta ImageMeta
	db.Where("id = ?", id).First(&meta)

	return &meta, nil
}

func (s *PostgreSQLStore) fetchMetaForHash(hash string) (*ImageMeta, error) {
	db, err := s.getConnection()
	if err != nil {
		return nil, err
	}
	defer db.Close()

	var meta ImageMeta
	db.Where("hash = ?", hash).First(&meta)

	return &meta, nil
}

func (s *PostgreSQLStore) getServiceStats() (*ServiceStats, error) {
	db, err := s.getConnection()
	if err != nil {
		return nil, err
	}
	defer db.Close()

	stats := ServiceStats{
		PopularFormat:         PopularFormat{},
		Top10CameraModels:     []CameraModel{},
		UploadFrequency30Days: []DayFrequency{},
	}

	db.Raw("SELECT media_type AS format, " +
		"count(*) AS uploads FROM image_meta " +
		"GROUP BY 1 ORDER BY 2 DESC LIMIT 1").Scan(&stats.PopularFormat)
	stats.PopularFormat.Format = strings.ToUpper(strings.TrimPrefix(stats.PopularFormat.Format, imageMediaType))

	db.Raw("SELECT camera_model AS model, " +
		"count(*) AS uploads FROM image_meta " +
		"GROUP BY 1 ORDER BY 2 DESC LIMIT 10").Find(&stats.Top10CameraModels)

	db.Raw("SELECT date_trunc('day', uploaded) AS date, " +
		"count(*) AS uploads FROM image_meta " +
		"WHERE uploaded > now() - interval '30 days' " +
		"GROUP BY 1 ORDER BY 1").Scan(&stats.UploadFrequency30Days)

	return &stats, nil
}

func (s *PostgreSQLStore) getConnection() (*gorm.DB, error) {
	return gorm.Open("postgres", s.url)
}
