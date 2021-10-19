package handlers_test

import (
	"github.com/davidjwilkins/declarative-payments/payments/handlers"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stripe/stripe-go/v72"
	"github.com/stripe/stripe-go/v72/client"
	"os"
	"strings"
	"testing"
)

func TestStripe(t *testing.T) {
	stripeKey := os.Getenv("sk_test")
	if !strings.HasPrefix(stripeKey, "sk_test") {
		panic("wtf")
		t.Skip()
		return
	}
	c := client.New(stripeKey, nil)
	stripe.DefaultLeveledLogger = &stripe.LeveledLogger{Level: 0}

	t.Run("Can charge", func(t *testing.T) {
		storage := handlers.NewMockStripeStorage("test")
		handler := handlers.NewStripeHandler(c, "tok_visa", string(stripe.CurrencyUSD), "test", storage)
		assert.Equal(t, storage.Balance(), uint(0))
		err := handler.Charge(uuid.New().String(), 1000)
		assert.NoError(t, err)
		assert.Equal(t, uint(1000), storage.Balance())
	})
	t.Run("Can refund and partial refund", func(t *testing.T) {
		storage := handlers.NewMockStripeStorage("test")
		handler := handlers.NewStripeHandler(c, "tok_visa", string(stripe.CurrencyUSD), "test", storage)
		assert.Equal(t, storage.Balance(), uint(0))
		err := handler.Charge(uuid.New().String(), 1000)
		assert.NoError(t, err)
		assert.Equal(t, uint(1000), storage.Balance())
		r, err := handler.Refund(uuid.New().String(), 500)
		assert.NoError(t, err)
		assert.Equal(t, uint(500), r)
		assert.Equal(t, 500, int(storage.Balance()))
		r, err = handler.Refund(uuid.New().String(), 500)
		assert.NoError(t, err)
		assert.Equal(t, uint(500), r)
		assert.Equal(t, uint(0), storage.Balance())
	})
	t.Run("Cannot over-refund", func(t *testing.T) {
		storage := handlers.NewMockStripeStorage("test")
		handler := handlers.NewStripeHandler(c, "tok_visa", string(stripe.CurrencyUSD), "test", storage)
		assert.Equal(t, storage.Balance(), uint(0))
		err := handler.Charge(uuid.New().String(), 1000)
		assert.NoError(t, err)
		assert.Equal(t, uint(1000), storage.Balance())
		r, err := handler.Refund(uuid.New().String(), 1001)
		assert.Error(t, err)
		assert.Equal(t, uint(0), r)
		assert.Equal(t, uint(1000), storage.Balance())
	})
	t.Run("Can authorize", func(t *testing.T) {
		storage := handlers.NewMockStripeStorage("test")
		handler := handlers.NewStripeHandler(c, "tok_visa", string(stripe.CurrencyUSD), "test", storage)
		assert.Equal(t, storage.Balance(), uint(0))
		err := handler.Authorize(uuid.New().String(), 1000)
		assert.NoError(t, err)
		assert.Equal(t, uint(1000), storage.AuthorizedBalance())
		assert.Equal(t, uint(0), storage.Balance())
	})
	t.Run("Can capture", func(t *testing.T) {
		storage := handlers.NewMockStripeStorage("test")
		handler := handlers.NewStripeHandler(c, "tok_visa", string(stripe.CurrencyUSD), "test", storage)
		assert.Equal(t, storage.Balance(), uint(0))
		err := handler.Authorize(uuid.New().String(), 1000)
		assert.NoError(t, err)
		assert.Equal(t, uint(0), storage.Balance())
		assert.Equal(t, uint(1000), storage.AuthorizedBalance())
		captured, err := handler.Capture(uuid.New().String(), 1000)
		assert.NoError(t, err)
		assert.Equal(t, uint(1000), storage.Balance())
		assert.Equal(t, uint(0), storage.AuthorizedBalance())
		assert.Equal(t, uint(1000), captured)
	})
	t.Run("Handles capture failure", func(t *testing.T) {
		storage := handlers.NewMockStripeStorage("test")
		handler := handlers.NewStripeHandler(c, "tok_visa", string(stripe.CurrencyUSD), "test", storage)
		assert.Equal(t, 0, int(storage.Balance()))
		err := handler.Authorize(uuid.New().String(), 1000)
		assert.NoError(t, err)
		err = handler.Authorize(uuid.New().String(), 1000)
		assert.NoError(t, err)
		err = handler.Authorize(uuid.New().String(), 100)
		assert.NoError(t, err)
		assert.Equal(t, 0, int(storage.Balance()))
		assert.Equal(t, 2100, int(storage.AuthorizedBalance()))
		// We want the last capture to fail, but our code makes it hard to actually do that, so we need to trick it
		// into doing something it wouldn't
		auths := storage.GetAuthorizationsFor(2100)
		for _, ch := range auths {
			if ch.Amount == 100 {
				ch.Amount = 1000
				storage.UpsertCharge(ch)
			}
		}
		assert.Equal(t, 3000, int(storage.AuthorizedBalance()))
		captured, err := handler.Capture(uuid.New().String(), 3000)
		assert.Error(t, err)
		assert.Equal(t, 2000, int(storage.Balance()))
		assert.Equal(t, 100, int(storage.AuthorizedBalance()))
		assert.Equal(t, 2000, int(captured))
	})
	t.Run("Can partial capture", func(t *testing.T) {
		storage := handlers.NewMockStripeStorage("test")
		handler := handlers.NewStripeHandler(c, "tok_visa", string(stripe.CurrencyUSD), "test", storage)
		assert.Equal(t, storage.Balance(), uint(0))
		err := handler.Authorize(uuid.New().String(), 1000)
		assert.NoError(t, err)
		assert.Equal(t, uint(0), storage.Balance())
		assert.Equal(t, uint(1000), storage.AuthorizedBalance())
		captured, err := handler.Capture(uuid.New().String(), 750)
		assert.NoError(t, err)
		assert.Equal(t, 750, int(storage.Balance()))
		assert.Equal(t, uint(250), storage.AuthorizedBalance())
		assert.Equal(t, uint(750), captured)
	})
	t.Run("Can release authorization", func(t *testing.T) {
		storage := handlers.NewMockStripeStorage("test")
		handler := handlers.NewStripeHandler(c, "tok_visa", string(stripe.CurrencyUSD), "test", storage)
		assert.Equal(t, 0, int(storage.Balance()))
		err := handler.Authorize(uuid.New().String(), 1000)
		assert.NoError(t, err)
		assert.Equal(t, 0, int(storage.Balance()))
		assert.Equal(t, 1000, int(storage.AuthorizedBalance()))
		err = handler.Authorize(uuid.New().String(), 1000)
		assert.NoError(t, err)
		assert.Equal(t, 0, int(storage.Balance()))
		assert.Equal(t, 2000, int(storage.AuthorizedBalance()))
		released, err := handler.Release(uuid.New().String(), 750)
		assert.NoError(t, err)
		assert.Equal(t, 0, int(storage.Balance()))
		assert.Equal(t, 1250, int(storage.AuthorizedBalance()))
		assert.Equal(t, 750, int(released))
		released, err = handler.Release(uuid.New().String(), 1250)
		assert.NoError(t, err)
		assert.Equal(t, 0, int(storage.Balance()))
		assert.Equal(t, 0, int(storage.AuthorizedBalance()))
		assert.Equal(t, 1250, int(released))
	})
	t.Run("Can capture and release simultaneously", func(t *testing.T) {
		storage := handlers.NewMockStripeStorage("test")
		handler := handlers.NewStripeHandler(c, "tok_visa", string(stripe.CurrencyUSD), "test", storage)
		assert.Equal(t, storage.Balance(), uint(0))
		err := handler.Authorize(uuid.New().String(), 1000)
		assert.NoError(t, err)
		assert.Equal(t, uint(0), storage.Balance())
		assert.Equal(t, uint(1000), storage.AuthorizedBalance())
		captured, captureErr, released, releaseErr := handler.CaptureRelease(uuid.New().String(), 750, uuid.New().String(), 250)
		assert.NoError(t, captureErr)
		assert.NoError(t, releaseErr)
		assert.Equal(t, 750, int(storage.Balance()))
		assert.Equal(t, 0, int(storage.AuthorizedBalance()))
		assert.Equal(t, 750, int(captured))
		assert.Equal(t, 250, int(released))
	})
	t.Run("Can capture and release with reauth", func(t *testing.T) {
		storage := handlers.NewMockStripeStorage("test")
		handler := handlers.NewStripeHandler(c, "tok_visa", string(stripe.CurrencyUSD), "test", storage)
		assert.Equal(t, storage.Balance(), uint(0))
		err := handler.Authorize(uuid.New().String(), 1000)
		assert.NoError(t, err)
		assert.Equal(t, uint(0), storage.Balance())
		assert.Equal(t, uint(1000), storage.AuthorizedBalance())
		captured, captureErr, released, releaseErr := handler.CaptureRelease(uuid.New().String(), 500, uuid.New().String(), 250)
		assert.NoError(t, captureErr)
		assert.NoError(t, releaseErr)
		assert.Equal(t, 500, int(storage.Balance()))
		assert.Equal(t, 250, int(storage.AuthorizedBalance()))
		assert.Equal(t, 500, int(captured))
		assert.Equal(t, 250, int(released))
	})
	t.Run("Can capture and release, reauth fails", func(t *testing.T) {
		storage := handlers.NewMockStripeStorage("test")
		handler, setCard := handlers.TestNewStripeHandler(c, "tok_visa", string(stripe.CurrencyUSD), "test", storage)
		assert.Equal(t, storage.Balance(), uint(0))
		err := handler.Authorize(uuid.New().String(), 1000)
		assert.NoError(t, err)
		assert.Equal(t, uint(0), storage.Balance())
		assert.Equal(t, uint(1000), storage.AuthorizedBalance())
		setCard("tok_chargeDeclinedInsufficientFunds")
		captured, captureErr, released, releaseErr := handler.CaptureRelease(uuid.New().String(), 500, uuid.New().String(), 250)
		assert.Error(t, captureErr)
		assert.NoError(t, releaseErr)
		assert.Equal(t, 500, int(storage.Balance()))
		assert.Equal(t, 0, int(storage.AuthorizedBalance()))
		assert.Equal(t, 500, int(captured))
		assert.Equal(t, 500, int(released))
	})
	t.Run("Can capture and release with additional release", func(t *testing.T) {
		storage := handlers.NewMockStripeStorage("test")
		handler := handlers.NewStripeHandler(c, "tok_visa", string(stripe.CurrencyUSD), "test", storage)
		assert.Equal(t, storage.Balance(), uint(0))
		err := handler.Authorize(uuid.New().String(), 1000)
		assert.NoError(t, err)
		err = handler.Authorize(uuid.New().String(), 1000)
		assert.NoError(t, err)
		assert.Equal(t, uint(0), storage.Balance())
		assert.Equal(t, uint(2000), storage.AuthorizedBalance())
		captured, captureErr, released, releaseErr := handler.CaptureRelease(uuid.New().String(), 1000, uuid.New().String(), 1000)
		assert.NoError(t, captureErr)
		assert.NoError(t, releaseErr)
		assert.Equal(t, 1000, int(storage.Balance()))
		assert.Equal(t, 0, int(storage.AuthorizedBalance()))
		assert.Equal(t, 1000, int(captured))
		assert.Equal(t, 1000, int(released))
	})
	t.Run("Can refund multiple Charges", func(t *testing.T) {
		storage := handlers.NewMockStripeStorage("test")
		handler := handlers.NewStripeHandler(c, "tok_visa", string(stripe.CurrencyUSD), "test", storage)
		assert.Equal(t, 0, int(storage.Balance()))
		err := handler.Charge(uuid.New().String(), 1000)
		assert.NoError(t, err)
		assert.Equal(t, 1000, int(storage.Balance()))
		assert.Equal(t, 0, int(storage.AuthorizedBalance()))
		err = handler.Charge(uuid.New().String(), 1000)
		assert.NoError(t, err)
		assert.Equal(t, 2000, int(storage.Balance()))
		assert.Equal(t, 0, int(storage.AuthorizedBalance()))
		refunded, err := handler.Refund(uuid.New().String(), 750)
		assert.NoError(t, err)
		assert.Equal(t, 1250, int(storage.Balance()))
		assert.Equal(t, 0, int(storage.AuthorizedBalance()))
		assert.Equal(t, 750, int(refunded))
		refunded, err = handler.Refund(uuid.New().String(), 1250)
		assert.NoError(t, err)
		assert.Equal(t, 0, int(storage.Balance()))
		assert.Equal(t, 0, int(storage.AuthorizedBalance()))
		assert.Equal(t, 1250, int(refunded))
	})
	t.Run("Error does not affect balance", func(t *testing.T) {
		storage := handlers.NewMockStripeStorage("test")
		handler := handlers.NewStripeHandler(
			c,
			"tok_chargeDeclinedInsufficientFunds",
			string(stripe.CurrencyUSD),
			"test",
			storage,
		)
		assert.Equal(t, 0, int(storage.Balance()))
		err := handler.Charge(uuid.New().String(), 1000)
		assert.Error(t, err)
		assert.Equal(t, 0, int(storage.Balance()))
	})
	t.Run("Error does not affect authorized balance", func(t *testing.T) {
		storage := handlers.NewMockStripeStorage("test")
		handler := handlers.NewStripeHandler(
			c,
			"tok_chargeDeclinedInsufficientFunds",
			string(stripe.CurrencyUSD),
			"test",
			storage,
		)
		assert.Equal(t, 0, int(storage.AuthorizedBalance()))
		err := handler.Authorize(uuid.New().String(), 1000)
		assert.Error(t, err)
		assert.Equal(t, 0, int(storage.AuthorizedBalance()))
	})
	t.Run("Error on reauth means captured full amount", func(t *testing.T) {
		storage := handlers.NewMockStripeStorage("test")
		handler, setCard := handlers.TestNewStripeHandler(
			c,
			"tok_visa",
			string(stripe.CurrencyUSD),
			"test",
			storage,
		)
		assert.Equal(t, 0, int(storage.AuthorizedBalance()))
		err := handler.Authorize(uuid.New().String(), 1000)
		assert.NoError(t, err)
		setCard("tok_chargeDeclinedInsufficientFunds")
		assert.Equal(t, 1000, int(storage.AuthorizedBalance()))
		captured, err := handler.Capture(uuid.NewString(), 750)
		assert.Error(t, err)
		assert.Equal(t, 750, int(captured))
		assert.Equal(t, 750, int(storage.Balance()))
		assert.Equal(t, 0, int(storage.AuthorizedBalance()))
	})
	t.Run("Error on reauth when releasing does not release", func(t *testing.T) {
		storage := handlers.NewMockStripeStorage("test")
		handler, setCard := handlers.TestNewStripeHandler(
			c,
			"tok_visa",
			string(stripe.CurrencyUSD),
			"test",
			storage,
		)
		assert.Equal(t, 0, int(storage.AuthorizedBalance()))
		err := handler.Authorize(uuid.New().String(), 1000)
		assert.NoError(t, err)
		err = handler.Authorize(uuid.New().String(), 1000)
		assert.NoError(t, err)
		setCard("tok_chargeDeclinedInsufficientFunds")
		assert.Equal(t, 2000, int(storage.AuthorizedBalance()))
		released, err := handler.Release(uuid.NewString(), 1750)
		assert.Error(t, err)
		assert.Equal(t, 1000, int(released))
		assert.Equal(t, 0, int(storage.Balance()))
		assert.Equal(t, 1000, int(storage.AuthorizedBalance()))
	})
}



