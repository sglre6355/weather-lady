package database

import (
	"context"
	"fmt"
	"time"

	"github.com/sglre6355/weather-lady/internal/domain"
	"gorm.io/gorm"
)

const referenceYear = 2000

// SubscriptionStore persists subscriptions using GORM.
type SubscriptionStore struct {
	db *gorm.DB
}

// NewSubscriptionStore initialises a SubscriptionStore backed by db.
func NewSubscriptionStore(db *gorm.DB) *SubscriptionStore {
	return &SubscriptionStore{db: db}
}

// AutoMigrate ensures the subscriptions table exists with the expected schema.
func (s *SubscriptionStore) AutoMigrate(ctx context.Context) error {
	if s == nil || s.db == nil {
		return fmt.Errorf("subscription store not initialised")
	}

	return s.db.WithContext(ctx).AutoMigrate(&subscriptionRecord{})
}

// Create persists the provided subscription.
func (s *SubscriptionStore) Create(ctx context.Context, subscription domain.Subscription) error {
	if s == nil || s.db == nil {
		return fmt.Errorf("subscription store not initialised")
	}

	record := subscriptionRecord{
		ChannelID:       subscription.ChannelID,
		GuildID:         subscription.GuildID,
		TimeOfDay:       timeOfDay(subscription.Time),
		URL:             subscription.URL,
		ElementSelector: subscription.ElementSelector,
		Message:         subscription.Message,
	}

	return s.db.WithContext(ctx).Create(&record).Error
}

// List returns every persisted subscription.
func (s *SubscriptionStore) List(ctx context.Context) ([]domain.Subscription, error) {
	if s == nil || s.db == nil {
		return nil, fmt.Errorf("subscription store not initialised")
	}

	var records []subscriptionRecord
	if err := s.db.WithContext(ctx).Find(&records).Error; err != nil {
		return nil, err
	}

	subscriptions := make([]domain.Subscription, 0, len(records))
	for _, record := range records {
		subscriptions = append(subscriptions, domain.Subscription{
			ChannelID:       record.ChannelID,
			GuildID:         record.GuildID,
			Time:            fromTimeOfDay(record.TimeOfDay),
			URL:             record.URL,
			ElementSelector: record.ElementSelector,
			Message:         record.Message,
		})
	}

	return subscriptions, nil
}

// DeleteByChannel removes every subscription stored against channelID and returns the number removed.
func (s *SubscriptionStore) DeleteByChannel(ctx context.Context, channelID string) (int, error) {
	if s == nil || s.db == nil {
		return 0, fmt.Errorf("subscription store not initialised")
	}

	result := s.db.WithContext(ctx).Where("channel_id = ?", channelID).Delete(&subscriptionRecord{})
	return int(result.RowsAffected), result.Error
}

type subscriptionRecord struct {
	ID              uint      `gorm:"primaryKey"`
	ChannelID       string    `gorm:"column:channel_id;size:128;not null;index:idx_subscriptions_channel"`
	GuildID         string    `gorm:"column:guild_id;size:128;not null"`
	TimeOfDay       time.Time `gorm:"column:time_of_day;type:time;not null"`
	URL             string    `gorm:"column:url;type:text;not null"`
	ElementSelector string    `gorm:"column:element_selector;type:text;not null"`
	Message         string    `gorm:"column:message;type:text;not null"`
	CreatedAt       time.Time `gorm:"column:created_at;autoCreateTime"`
	UpdatedAt       time.Time `gorm:"column:updated_at;autoUpdateTime"`
}

func (subscriptionRecord) TableName() string {
	return "subscriptions"
}

func timeOfDay(input time.Time) time.Time {
	loc := input.Location()
	if loc == nil {
		loc = time.UTC
	}
	return time.Date(
		referenceYear,
		time.January,
		1,
		input.Hour(),
		input.Minute(),
		input.Second(),
		0,
		loc,
	)
}

func fromTimeOfDay(stored time.Time) time.Time {
	loc := stored.Location()
	if loc == nil {
		loc = time.UTC
	}
	return time.Date(
		referenceYear,
		time.January,
		1,
		stored.Hour(),
		stored.Minute(),
		stored.Second(),
		0,
		loc,
	)
}
