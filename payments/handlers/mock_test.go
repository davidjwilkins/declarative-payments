package handlers_test

import (
	"errors"
	"github.com/davidjwilkins/declarative-payments/payments/handlers"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"sync"
	"testing"
)

func TestPartnerMock(t *testing.T) {
	t.Run("Mock can be created", func(t *testing.T) {
		m := handlers.NewPartnerMock()
		assert.Equal(t, m.Balance(), 0, "Initial balance is 0")
	})
	t.Run("Test Deposit", func(t *testing.T) {
		m := handlers.NewPartnerMock()
		m.ShouldErr("abc", errors.New("test"))
		assert.Error(t, m.Deposit("abc", 100), "Can force a deposit error")
		assert.NoError(t, m.Deposit("abc", 100), "Can deposit successfully")
		assert.Equal(t, 100, m.Balance(), "Balance increases after deposit")
		assert.NoError(t, m.Deposit("abc", 100), "Deposit is successful on duplicate")
		assert.Equal(t, 100, m.Balance(), "Balance does not increases after duplicate deposit")

	})
	t.Run("Test Withdrawal", func(t *testing.T) {
		m := handlers.NewPartnerMock()
		m.ShouldErr("abc", errors.New("test"))
		assert.Error(t, m.Withdraw("abc", 100), "Can force a withdrawal error")
		assert.NoError(t, m.Withdraw("abc", 100), "Can withdraw successfully")
		assert.Equal(t, -100, m.Balance(), "Balance decreases after withdrawal")
		assert.NoError(t, m.Withdraw("abc", 100), "Withdrawal is successful on duplicate")
		assert.Equal(t, -100, m.Balance(), "Balance does not decrease after duplicate withdrawal")
	})
	t.Run("Test Concurrency", func(t *testing.T) {
		m := handlers.NewPartnerMock()
		var wg sync.WaitGroup
		wg.Add(10000)
		for i := 0; i < 10000; i++ {
			go func(i int) {
				defer wg.Done()
				assert.NoError(t, m.Withdraw(uuid.New().String(), 100), "Can withdraw concurrently")
				assert.NoError(t, m.Deposit(uuid.New().String(), 100), "Can deposit concurrently")
			}(i)
		}
		wg.Wait()
		assert.Equal(t, 0, m.Balance(), "Balance is correct after concurrent operations")
	})
}

func TestUserMock(t *testing.T) {
	t.Run("Mock can be created", func(t *testing.T) {
		m := handlers.NewUserMock()
		assert.Equal(t, m.Balance(), 0, "Initial balance is 0")
	})
	t.Run("Test Authorize", func(t *testing.T) {
		m := handlers.NewUserMock()
		m.ShouldErr("abc", errors.New("test"))
		assert.Error(t, m.Authorize("abc", 100), "Can force an authorization error")
		assert.NoError(t, m.Authorize("abc", 100), "Can authorize successfully")
		assert.Equal(t, uint(100), m.AuthorizedBalance(), "Balance increases after deposit")
		assert.NoError(t, m.Authorize("abc", 100), "Authorize is successful on duplicate")
		assert.Equal(t, uint(100), m.AuthorizedBalance(), "Balance does not increases after duplicate deposit")
	})
	t.Run("Test Capture", func(t *testing.T) {
		m := handlers.NewUserMock()
		amount, err := m.Capture("capture", 100)
		assert.Error(t, err, "Cannot capture more than authorized")
		assert.Equal(t, 0, int(amount))
		assert.NoError(t, m.Authorize("authorize", 100), "Can authorize successfully")
		assert.Equal(t, uint(100), m.AuthorizedBalance(), "Authorized balance increases after authorization")
		m.ShouldErr("capture", errors.New("test"))
		assert.Error(t, m.Authorize("capture", 100), "Can force a capture error")
		amount, err = m.Capture("capture", 100)
		assert.NoError(t, err, "Can capture replayed item after error")
		assert.Equal(t, 100, int(amount))
		assert.Equal(t, uint(0), m.AuthorizedBalance(), "Authorized balance decreases after capture")
		assert.Equal(t, 100, m.Balance(), "Balance increases after capture")
	})
	t.Run("Test Release", func(t *testing.T) {
		m := handlers.NewUserMock()
		released, err := m.Release("release", 100)
		assert.Error(t, err, "Cannot release more than authorized")
		assert.Equal(t, 0, int(released))
		assert.NoError(t, m.Authorize("authorize", 100), "Can authorize successfully")
		assert.Equal(t, uint(100), m.AuthorizedBalance(), "Authorized balance increases after authorization")
		m.ShouldErr("release", errors.New("test"))
		released, err = m.Release("release", 100)
		assert.Error(t, err, "Can force a release error")
		assert.Equal(t, 0, int(released))
		released, err = m.Release("release", 100)
		assert.NoError(t, err, "Can release replayed item after error")
		assert.Equal(t, 100, int(released))
		assert.Equal(t, uint(0), m.AuthorizedBalance(), "Authorized balance decreases after release")
		assert.Equal(t, 0, m.Balance(), "Balance does not increases after release")
	})
	t.Run("Test CaptureRelease", func(t *testing.T) {
		m := handlers.NewUserMock()
		captured, captureErr, released, releaseErr := m.CaptureRelease("release", 100, "capture", 100)
		assert.Error(t, captureErr, "Cannot release more than authorized")
		assert.Error(t, releaseErr, "Cannot release more than authorized")
		assert.Equal(t, 0, int(captured))
		assert.Equal(t, 0, int(released))
		assert.NoError(t, m.Authorize("authorize", 200), "Can authorize successfully")
		assert.Equal(t, uint(200), m.AuthorizedBalance(), "Authorized balance increases after authorization")
		m.ShouldErr("capture", errors.New("test"))
		m.ShouldErr("release", errors.New("test"))
		captured, captureErr, released, releaseErr = m.CaptureRelease("release", 100, "capture", 100)
		assert.Error(t, captureErr, "Can force a capture-release capture error")
		assert.Error(t, releaseErr, "Can force a capture-release release error")
		assert.Equal(t, 0, int(captured))
		assert.Equal(t, 0, int(released))
		captured, captureErr, released, releaseErr = m.CaptureRelease("release", 100, "capture", 100)
		assert.NoError(t, captureErr, "Can capture replayed item in captureRelease after error")
		assert.NoError(t, releaseErr, "Can release replayed item in captureRelease after error")
		assert.Equal(t, 100, int(captured))
		assert.Equal(t, 100, int(released))
		assert.Equal(t, uint(0), m.AuthorizedBalance(), "Authorized balance decreases after capture release")
		assert.Equal(t, 100, m.Balance(), "Balance increases by correct amount in capture release")
	})
	t.Run("Test Charge", func(t *testing.T) {
		m := handlers.NewUserMock()
		m.ShouldErr("abc", errors.New("test"))
		assert.Error(t, m.Charge("abc", 100), "Can force a charge error")
		assert.NoError(t, m.Charge("abc", 100), "Can charge")
		assert.Equal(t, 100, m.Balance(), "Balance increases after charge")
		assert.NoError(t, m.Charge("abc", 100), "Duplicate charge is successful")
		assert.Equal(t, 100, m.Balance(), "Balance does not increases after duplicate charge")
	})
	t.Run("Test Refund", func(t *testing.T) {
		m := handlers.NewUserMock()
		m.ShouldErr("refund", errors.New("test"))
		refunded, err := m.Refund("refund", 100)
		assert.Error(t, err, "Can force a refund error")
		assert.Equal(t, 0, int(refunded))
		refunded, err = m.Refund("refund", 100)
		assert.NoError(t, err, "Can refund more than charged")
		assert.Equal(t, 100, int(refunded))
		assert.Equal(t, -100, m.Balance(), "Balance decreases after refund")
		refunded, err = m.Refund("refund", 100)
		assert.NoError(t, err, "Duplicate refund is successful")
		assert.Equal(t, 100, int(refunded))
		assert.Equal(t, -100, m.Balance(), "Balance does not decrease after duplicate refund")
	})
	t.Run("Test Concurrency", func(t *testing.T) {
		m := handlers.NewUserMock()
		var wg sync.WaitGroup
		wg.Add(10000)
		for i := 0; i < 10000; i++ {
			go func(i int) {
				defer wg.Done()
				assert.NoError(t, m.Authorize(uuid.New().String(), 200), "Can authorize concurrently")
				captured, err := m.Capture(uuid.New().String(), 100)
				assert.NoError(t, err, "Can capture concurrently")
				assert.Equal(t, 100, int(captured))
				released, err := m.Release(uuid.New().String(), 100)
				assert.NoError(t, err, "Can release concurrently")
				assert.Equal(t, 100, int(released))
				assert.NoError(t, m.Charge(uuid.New().String(), 100), "Can charge concurrently")
				refunded, err := m.Refund(uuid.New().String(), 200)
				assert.NoError(t, err, "Can refund concurrently")
				assert.Equal(t, 200, int(refunded))
				errID := uuid.New().String()
				m.ShouldErr(errID, errors.New("test"))
				captured, err = m.Capture(errID, 100)
				assert.Error(t, err, "Can force errors concurrently")
				assert.Equal(t, 0, int(captured))
			}(i)
		}
		wg.Wait()
		assert.Equal(t, 0, m.Balance(), "Balance is correct after concurrent operations")
	})
}
