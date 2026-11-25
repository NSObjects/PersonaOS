package providerconfig

import (
	"context"
	"encoding/json"
	"errors"
	"os"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"gorm.io/gorm/logger"
)

// storedService is private to the provider configuration module.
type storedService struct {
	Service string `gorm:"primaryKey;size:64"`
	Config  string `gorm:"type:text;not null"`
}

func (storedService) TableName() string { return "provider_configs" }

// Store owns persistence and legacy environment fallback.
type Store struct {
	db     *gorm.DB
	getenv func(string) string
}

// NewStore creates the private Demo configuration table. SQL logging is disabled
// for this store because its values contain provider credentials.
func NewStore(ctx context.Context, db *gorm.DB) (*Store, error) {
	if db == nil {
		return nil, errors.New("provider configuration requires MySQL")
	}
	if err := db.WithContext(ctx).AutoMigrate(&storedService{}); err != nil {
		return nil, errors.New("cannot migrate provider configuration storage")
	}
	return &Store{db: db.Session(&gorm.Session{Logger: logger.Default.LogMode(logger.Silent)}), getenv: os.Getenv}, nil
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
	values, err := decodeValues(row.Config)
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
	data, err := encodeValues(normalized)
	if err != nil {
		return err
	}
	row := storedService{Service: id, Config: data}
	if err := s.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "service"}},
		DoUpdates: clause.AssignmentColumns([]string{"config"}),
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

// Update applies explicit changes to one service in a transaction. Existing
// snapshots are locked; concurrent first writes fail instead of overwriting
// an independently configured credential.
func (s *Store) Update(ctx context.Context, id string, changes Values) (View, error) {
	service, ok := Lookup(id)
	if !ok {
		return View{}, errors.New("unsupported provider service")
	}
	var view View
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var row storedService
		err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Take(&row, "service = ?", id).Error
		missing := errors.Is(err, gorm.ErrRecordNotFound)
		if err != nil && !missing {
			return errors.New("cannot read provider configuration")
		}
		current := service.Environment(s.getenv)
		if !missing {
			current, err = decodeValues(row.Config)
			if err != nil {
				return err
			}
		}
		next, err := service.Merge(current, changes)
		if err != nil {
			return err
		}
		data, err := encodeValues(next)
		if err != nil {
			return err
		}
		row = storedService{Service: id, Config: data}
		if missing {
			err = tx.Create(&row).Error
		} else {
			err = tx.Model(&storedService{}).Where("service = ?", id).Update("config", data).Error
		}
		if err != nil {
			return errors.New("cannot persist provider configuration")
		}
		view = service.Project(next, "database")
		return nil
	})
	if err != nil {
		return View{}, err
	}
	return view, nil
}

func encodeValues(values Values) (string, error) {
	data, err := json.Marshal(values)
	if err != nil {
		return "", errors.New("cannot encode provider configuration")
	}
	return string(data), nil
}

func decodeValues(data string) (Values, error) {
	var values Values
	if err := json.Unmarshal([]byte(data), &values); err != nil || values == nil {
		return nil, errors.New("invalid stored provider configuration")
	}
	return values, nil
}
