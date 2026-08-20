package checkhealth

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/khanzadimahdi/testproject/infrastructure/health"
)

func TestUseCase_Execute(t *testing.T) {
	t.Parallel()

	t.Run("all dependencies answer", func(t *testing.T) {
		t.Parallel()

		var (
			database  health.MockPinger
			messaging health.MockPinger
		)

		database.On("Ping", mock.Anything).Once().Return(nil)
		defer database.AssertExpectations(t)

		messaging.On("Ping", mock.Anything).Once().Return(nil)
		defer messaging.AssertExpectations(t)

		usecase := NewUseCase(
			Dependency{Name: "database", Pinger: &database},
			Dependency{Name: "messaging", Pinger: &messaging},
		)

		assert.NoError(t, usecase.Execute(context.Background()))
	})

	t.Run("no dependencies", func(t *testing.T) {
		t.Parallel()

		usecase := NewUseCase()

		assert.NoError(t, usecase.Execute(context.Background()))
	})

	t.Run("database doesn't answer", func(t *testing.T) {
		t.Parallel()

		var (
			database  health.MockPinger
			messaging health.MockPinger

			expectedErr = errors.New("connection refused")
		)

		database.On("Ping", mock.Anything).Once().Return(expectedErr)
		defer database.AssertExpectations(t)

		// the remaining dependencies are not probed once one of them fails
		defer messaging.AssertExpectations(t)

		usecase := NewUseCase(
			Dependency{Name: "database", Pinger: &database},
			Dependency{Name: "messaging", Pinger: &messaging},
		)

		err := usecase.Execute(context.Background())

		assert.ErrorIs(t, err, expectedErr)
		assert.EqualError(t, err, "database: connection refused")
	})

	t.Run("messaging doesn't answer", func(t *testing.T) {
		t.Parallel()

		var (
			database  health.MockPinger
			messaging health.MockPinger

			expectedErr = errors.New("nats: server is disconnected")
		)

		database.On("Ping", mock.Anything).Once().Return(nil)
		defer database.AssertExpectations(t)

		messaging.On("Ping", mock.Anything).Once().Return(expectedErr)
		defer messaging.AssertExpectations(t)

		usecase := NewUseCase(
			Dependency{Name: "database", Pinger: &database},
			Dependency{Name: "messaging", Pinger: &messaging},
		)

		err := usecase.Execute(context.Background())

		assert.ErrorIs(t, err, expectedErr)
		assert.EqualError(t, err, "messaging: nats: server is disconnected")
	})
}
