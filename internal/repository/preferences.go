package repository

import (
	"context"
	"fmt"
	"sort"

	"skipjd/internal/gamejob"
	"skipjd/internal/model"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type PreferencesRepository struct {
	db *gorm.DB
}

func NewPreferencesRepository(db *gorm.DB) *PreferencesRepository {
	return &PreferencesRepository{db: db}
}

func (r *PreferencesRepository) EnsureUser(ctx context.Context, id uint, email string) (*model.User, error) {
	if id == 0 {
		return nil, fmt.Errorf("ensure user: id must be > 0")
	}

	if err := r.db.WithContext(ctx).
		Clauses(clause.OnConflict{DoNothing: true}).
		Create(&model.User{
			ID:       id,
			Email:    email,
			Password: "dummy_password",
			Name:     "Test User",
		}).
		Error; err != nil {
		return nil, fmt.Errorf("ensure user: %w", err)
	}

	var user model.User
	if err := r.db.WithContext(ctx).First(&user, id).Error; err != nil {
		return nil, fmt.Errorf("ensure user reload: %w", err)
	}
	return &user, nil
}

func (r *PreferencesRepository) ListDistinctCompanyNames(ctx context.Context) ([]string, error) {
	rows := make([]struct{ Company string }, 0)
	if err := r.db.WithContext(ctx).
		Model(&model.JobPosting{}).
		Distinct("company").
		Where("company <> ''").
		Find(&rows).
		Error; err != nil {
		return nil, fmt.Errorf("list distinct company names: %w", err)
	}

	seen := make(map[string]struct{}, len(rows))
	names := make([]string, 0, len(rows))
	for _, row := range rows {
		normalized := gamejob.NormalizeCompanyName(row.Company)
		if normalized == "" {
			continue
		}
		if _, exists := seen[normalized]; exists {
			continue
		}
		seen[normalized] = struct{}{}
		names = append(names, normalized)
	}

	sort.Strings(names)
	return names, nil
}

func (r *PreferencesRepository) ReplaceUserDutyPreferences(ctx context.Context, userID uint, dutyCodes []int) error {
	if userID == 0 {
		return fmt.Errorf("replace user duty preferences: userID must be > 0")
	}

	normalized := gamejob.NormalizeDutyCodes(dutyCodes)

	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.
			Where("user_id = ?", userID).
			Delete(&model.UserDutyPreference{}).
			Error; err != nil {
			return err
		}

		if len(normalized) == 0 {
			return nil
		}

		rows := make([]model.UserDutyPreference, 0, len(normalized))
		for _, code := range normalized {
			rows = append(rows, model.UserDutyPreference{UserID: userID, DutyCode: code})
		}
		return tx.Create(&rows).Error
	})
}

func (r *PreferencesRepository) ReplaceUserCompanyPreferences(ctx context.Context, userID uint, companies []string) error {
	if userID == 0 {
		return fmt.Errorf("replace user company preferences: userID must be > 0")
	}

	seen := make(map[string]struct{}, len(companies))
	normalized := make([]string, 0, len(companies))
	for _, raw := range companies {
		name := gamejob.NormalizeCompanyName(raw)
		if name == "" {
			continue
		}
		if _, exists := seen[name]; exists {
			continue
		}
		seen[name] = struct{}{}
		normalized = append(normalized, name)
	}

	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.
			Where("user_id = ?", userID).
			Delete(&model.UserCompanyPreference{}).
			Error; err != nil {
			return err
		}

		if len(normalized) == 0 {
			return nil
		}

		rows := make([]model.UserCompanyPreference, 0, len(normalized))
		for _, name := range normalized {
			rows = append(rows, model.UserCompanyPreference{UserID: userID, CompanyName: name})
		}
		return tx.Create(&rows).Error
	})
}

func (r *PreferencesRepository) GetUserDutyCodes(ctx context.Context, userID uint) ([]int, error) {
	rows := make([]model.UserDutyPreference, 0)
	if err := r.db.WithContext(ctx).
		Where("user_id = ?", userID).
		Order("duty_code ASC").
		Find(&rows).
		Error; err != nil {
		return nil, fmt.Errorf("get user duty codes: %w", err)
	}

	codes := make([]int, 0, len(rows))
	for _, row := range rows {
		codes = append(codes, row.DutyCode)
	}
	return codes, nil
}

func (r *PreferencesRepository) GetUserCompanyNames(ctx context.Context, userID uint) ([]string, error) {
	rows := make([]model.UserCompanyPreference, 0)
	if err := r.db.WithContext(ctx).
		Where("user_id = ?", userID).
		Order("company_name ASC").
		Find(&rows).
		Error; err != nil {
		return nil, fmt.Errorf("get user company names: %w", err)
	}

	names := make([]string, 0, len(rows))
	for _, row := range rows {
		names = append(names, row.CompanyName)
	}
	return names, nil
}
