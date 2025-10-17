package usecase

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/sglre6355/weather-lady/internal/domain"
)

// ForecastCapture exposes the ability to render a forecast snapshot for a given source.
type ForecastCapture interface {
	CaptureForecast(ctx context.Context, url, elementSelector string) ([]byte, error)
}

// ForecastSender delivers a rendered forecast to the desired destination.
type ForecastSender interface {
	SendForecast(ctx context.Context, channelID string, imageData []byte, message string) error
}

// SubscriptionErrorStage indicates which step of the delivery pipeline failed.
type SubscriptionErrorStage string

const (
	// SubscriptionErrorStageCapture marks failures while capturing the weather snapshot.
	SubscriptionErrorStageCapture SubscriptionErrorStage = "capture"
	// SubscriptionErrorStageDispatch marks failures while dispatching the snapshot to the consumer.
	SubscriptionErrorStageDispatch SubscriptionErrorStage = "dispatch"
)

// SubscriptionErrorHandler is invoked when a scheduled run cannot complete successfully.
type SubscriptionErrorHandler func(domain.Subscription, SubscriptionErrorStage, error)

type subscriptionEntry struct {
	subscription domain.Subscription
	stopChan     chan struct{}
}

// SubscriptionManager coordinates scheduled forecast deliveries for channels.
type SubscriptionManager struct {
	mu            sync.RWMutex
	subscriptions map[string][]*subscriptionEntry

	capture ForecastCapture
	sender  ForecastSender

	nowFn           func() time.Time
	interval        time.Duration
	captureTimeout  time.Duration
	dispatchTimeout time.Duration
	onError         SubscriptionErrorHandler
}

// SubscriptionManagerOption configures behavioural aspects of the scheduler.
type SubscriptionManagerOption func(*SubscriptionManager)

// WithSubscriptionClock overrides the clock used to determine the next dispatch instant (useful for testing).
func WithSubscriptionClock(nowFn func() time.Time) SubscriptionManagerOption {
	return func(m *SubscriptionManager) {
		if nowFn != nil {
			m.nowFn = nowFn
		}
	}
}

// WithSubscriptionInterval defines the cadence between deliveries.
func WithSubscriptionInterval(interval time.Duration) SubscriptionManagerOption {
	return func(m *SubscriptionManager) {
		if interval > 0 {
			m.interval = interval
		}
	}
}

// WithCaptureTimeout customises the maximum duration allowed for snapshot rendering.
func WithCaptureTimeout(timeout time.Duration) SubscriptionManagerOption {
	return func(m *SubscriptionManager) {
		if timeout > 0 {
			m.captureTimeout = timeout
		}
	}
}

// WithDispatchTimeout customises the maximum duration allowed for dispatching a snapshot.
func WithDispatchTimeout(timeout time.Duration) SubscriptionManagerOption {
	return func(m *SubscriptionManager) {
		if timeout > 0 {
			m.dispatchTimeout = timeout
		}
	}
}

// WithSubscriptionErrorHandler registers the callback used when a dispatch cycle fails.
func WithSubscriptionErrorHandler(handler SubscriptionErrorHandler) SubscriptionManagerOption {
	return func(m *SubscriptionManager) {
		if handler != nil {
			m.onError = handler
		}
	}
}

// NewSubscriptionManager builds a manager that captures forecasts via capture and dispatches via sender.
func NewSubscriptionManager(
	capture ForecastCapture,
	sender ForecastSender,
	opts ...SubscriptionManagerOption,
) *SubscriptionManager {
	manager := &SubscriptionManager{
		subscriptions:   make(map[string][]*subscriptionEntry),
		capture:         capture,
		sender:          sender,
		nowFn:           time.Now,
		interval:        24 * time.Hour,
		captureTimeout:  30 * time.Second,
		dispatchTimeout: 30 * time.Second,
		onError:         func(domain.Subscription, SubscriptionErrorStage, error) {},
	}

	for _, opt := range opts {
		opt(manager)
	}

	return manager
}

// Add registers a new subscription and starts its delivery schedule.
func (m *SubscriptionManager) Add(sub domain.Subscription) error {
	if m.capture == nil {
		return fmt.Errorf("subscription manager missing forecast capture dependency")
	}
	if m.sender == nil {
		return fmt.Errorf("subscription manager missing forecast sender dependency")
	}

	entry := &subscriptionEntry{
		subscription: sub,
		stopChan:     make(chan struct{}),
	}

	m.mu.Lock()
	m.subscriptions[sub.ChannelID] = append(m.subscriptions[sub.ChannelID], entry)
	m.mu.Unlock()

	go m.schedule(entry)
	return nil
}

// Remove cancels all subscriptions for a channel and returns how many were removed.
func (m *SubscriptionManager) Remove(channelID string) int {
	m.mu.Lock()
	entries, ok := m.subscriptions[channelID]
	if !ok {
		m.mu.Unlock()
		return 0
	}
	delete(m.subscriptions, channelID)
	m.mu.Unlock()

	for _, entry := range entries {
		close(entry.stopChan)
	}

	return len(entries)
}

// Shutdown cancels every active subscription. Returns total number cancelled.
func (m *SubscriptionManager) Shutdown() int {
	m.mu.Lock()
	toStop := m.subscriptions
	m.subscriptions = make(map[string][]*subscriptionEntry)
	m.mu.Unlock()

	total := 0
	for _, entries := range toStop {
		total += len(entries)
		for _, entry := range entries {
			close(entry.stopChan)
		}
	}

	return total
}

func (m *SubscriptionManager) schedule(entry *subscriptionEntry) {
	nextRun := m.nextRun(entry.subscription.Time)
	timer := time.NewTimer(time.Until(nextRun))
	defer timer.Stop()

	for {
		select {
		case <-timer.C:
			if err := m.captureAndSend(entry.subscription); err != nil {
				timer.Reset(m.interval)
				continue
			}
			timer.Reset(m.interval)
		case <-entry.stopChan:
			return
		}
	}
}

func (m *SubscriptionManager) captureAndSend(sub domain.Subscription) error {
	ctxCapture, cancelCapture := context.WithTimeout(context.Background(), m.captureTimeout)
	imageData, err := m.capture.CaptureForecast(ctxCapture, sub.URL, sub.ElementSelector)
	cancelCapture()
	if err != nil {
		m.onError(
			sub,
			SubscriptionErrorStageCapture,
			fmt.Errorf("failed to capture forecast: %w", err),
		)
		return err
	}

	ctxSend, cancelSend := context.WithTimeout(context.Background(), m.dispatchTimeout)
	defer cancelSend()
	if err := m.sender.SendForecast(ctxSend, sub.ChannelID, imageData, sub.Message); err != nil {
		m.onError(
			sub,
			SubscriptionErrorStageDispatch,
			fmt.Errorf("failed to dispatch forecast: %w", err),
		)
		return err
	}

	return nil
}

func (m *SubscriptionManager) nextRun(target time.Time) time.Time {
	now := m.nowFn()
	scheduled := time.Date(
		now.Year(),
		now.Month(),
		now.Day(),
		target.Hour(),
		target.Minute(),
		0,
		0,
		now.Location(),
	)

	if scheduled.After(now) {
		return scheduled
	}

	return scheduled.Add(m.interval)
}
