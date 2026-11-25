package providerconfig

import (
	"context"
	"errors"
	"os"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// storedService contains ciphertext only; it is not a system config record.
type storedService struct {
	Service    string `gorm:"primaryKey;size:64"`
	Ciphertext []byte `gorm:"type:blob;not null"`
}

func (storedService) TableName() string { return "provider_service_configs" }

// Store owns encrypted persistence and legacy environment fallback.
type Store struct {
	db     *gorm.DB
	cipher *Cipher
	getenv func(string) string
}

// NewStore permits env-only deployments without an encryption key. Persisted
// credentials cannot be read or written without the configured deployment key.
func NewStore(ctx context.Context, db *gorm.DB, cipher *Cipher) (*Store, error) {
	if db == nil {
		return nil, errors.New("provider configuration requires MySQL")
	}
	if err := db.WithContext(ctx).AutoMigrate(&storedService{}); err != nil {
		return nil, errors.New("cannot migrate provider configuration storage")
	}
	return &Store{db: db, cipher: cipher, getenv: os.Getenv}, nil
}

// Load returns the whole stored group or its environment fallback. A corrupt
// row is an error, never an excuse to silently use old environment credentials.
func (s *Store) Load(ctx context.Context, id string) (Values, string, error) {
	service, ok := Lookup(id)
	if !ok {
		return nil, "", errors.New("unsupported provider service")
	}
	var row storedService
	err := s.db.WithContext(ctx).Take(&row, "service = ?", id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return service.Environment(s.getenv), "environment", nil
	}
	if err != nil {
		return nil, "", errors.New("cannot read provider configuration")
	}
	values, err := s.cipher.Open(id, row.Ciphertext)
	if err != nil {
		return nil, "", err
	}
	return values, "database", nil
}

// Replace persists a complete validated snapshot; even an empty credential
// stays explicitly cleared instead of reverting to the environment.
func (s *Store) Replace(ctx context.Context, id string, values Values) error {
	service, ok := Lookup(id)
	if !ok {
		return errors.New("unsupported provider service")
	}
	normalized, err := service.Merge(nil, values)
	if err != nil {
		return err
	}
	data, err := s.cipher.Seal(id, normalized)
	if err != nil {
		return err
	}
	row := storedService{Service: id, Ciphertext: data}
	if err := s.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "service"}},
		DoUpdates: clause.AssignmentColumns([]string{"ciphertext"}),
	}).Create(&row).Error; err != nil {
		return errors.New("cannot persist provider configuration")
	}
	return nil
}

// List exposes redacted views only, including the source of each group.
func (s *Store) List(ctx context.Context) ([]View, error) {
	views := make([]View, 0, len(Services()))
	for _, service := range Services() {
		values, source, err := s.Load(ctx, service.ID)
		if err != nil {
			return nil, err
		}
		views = append(views, service.Project(values, source))
	}
	return views, nil
}
