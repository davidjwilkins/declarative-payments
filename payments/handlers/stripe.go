package handlers

import (
	"github.com/stripe/stripe-go/v72"
	"github.com/stripe/stripe-go/v72/client"
)

type StripeStorage interface {
	ListAuthorizations() []stripe.Charge
	GetAuthorizationsFor(amount uint) []stripe.Charge
	GetChargesFor(amount uint) []stripe.Charge
	UpsertCharge(ch stripe.Charge)
}
type stripeHandler struct {
	*client.API
	cardID string
	bucket string
	currency stripe.Currency
	storage StripeStorage
}

func (s stripeHandler) doCharge(authorization bool, idempotencyKey string, amount uint) error {
	ch, err := s.Charges.New(&stripe.ChargeParams{
		Amount: stripe.Int64(int64(amount)),
		Capture: stripe.Bool(!authorization),
		Source: &stripe.SourceParams{Token: stripe.String(s.cardID)},
		Currency: stripe.String(string(s.currency)),
		Params: stripe.Params{
			IdempotencyKey: stripe.String(idempotencyKey),
			Metadata: map[string]string{
				"bucket": s.bucket,
				"idempotencyKey": idempotencyKey,
			},
		},
	})
	if ch != nil && ch.ID != "" {
		s.storage.UpsertCharge(*ch)
	}
	if err != nil {
		return err
	}
	return nil
}

func (s stripeHandler) Authorize(idempotencyKey string, amount uint) error {
	return s.doCharge(true, idempotencyKey, amount)
}

// doCapture will capture authorized amounts, and return how much was captured, and how much was released by capturing
func (s stripeHandler) doCapture(idempotencyKey string, amount uint) (uint, uint, error) {
	auths := s.storage.GetAuthorizationsFor(amount)
	amountLeft := int64(amount)
	totalCaptured := uint(0)
	totalReleased := uint(0)
	var lastErr error
	for i, auth := range auths {
		captureAmount := auth.Amount - auth.AmountRefunded
		if i == len(auths) - 1 {
			captureAmount = amountLeft
		}
		amountLeft -= captureAmount
		ch, err := s.Charges.Capture(auth.ID, &stripe.CaptureParams{
			Amount: stripe.Int64(captureAmount),
			Params: stripe.Params{
				IdempotencyKey: stripe.String(idempotencyKey + ":" + auth.ID),
			},
		})
		if ch != nil && ch.ID != "" {
			s.storage.UpsertCharge(*ch)
		}
		if err == nil && ch.Status != "failed" {
			totalCaptured += uint(captureAmount)
			if i == len(auths) - 1 {
				totalReleased = uint(auth.Amount - auth.AmountRefunded - captureAmount)
			}
		}
		if err != nil {
			lastErr = err
			// Refresh the charge, our data might be stale and this would be a good time to update
			ch, err := s.Charges.Get(auth.ID, nil)
			if err == nil && ch != nil && ch.ID == auth.ID {
				s.storage.UpsertCharge(*ch)
			}
		}
	}
	return totalCaptured, totalReleased, lastErr
}

// Capture will capture authorized amounts, re-authorizing any amount released. It returns the amount successfully
// captured, and an error.
func (s stripeHandler) Capture(idempotencyKey string, amount uint) (uint, error) {
	totalCaptured, totalReleased, err := s.doCapture(idempotencyKey, amount)
	if totalReleased > 0 {
		reauthErr := s.Authorize(idempotencyKey + ":reauthorize", totalReleased)
		if reauthErr == nil {
			totalReleased = 0
		}
		if reauthErr != nil && err == nil {
			err = reauthErr
		}
	}
	return totalCaptured, err
}

func (s stripeHandler) doRelease(charges []stripe.Charge, idempotencyKey string, amount uint) (uint, error) {
	amountLeft := int64(amount)
	totalReleased := uint(0)
	var lastErr error
	for i, auth := range charges {
		releaseAmount := auth.Amount - auth.AmountRefunded
		if i == len(charges) - 1 {
			if auth.Captured {
				releaseAmount = amountLeft
			} else {
				// Stripe does not let you partially release an authorization, so in this case we need to re-authorize
				// the difference
				if releaseAmount > amountLeft {
					err := s.Authorize(idempotencyKey + ":reauth", uint(releaseAmount - amountLeft))
					if err != nil {
						// If we couldn't reauthorize the amount, then it's better to have too much authorized than too
						// little, so bail now
						return totalReleased, err
					}
					// We've reauthorized, so we're "released" less
					totalReleased -= uint(releaseAmount - amountLeft)
				}

			}

		}
		amountLeft -= releaseAmount
		refund, err := s.Refunds.New(&stripe.RefundParams{
			Amount: stripe.Int64(releaseAmount),
			Charge: stripe.String(auth.ID),
			Params: stripe.Params{
				Expand: []*string{stripe.String("charge")},
				IdempotencyKey: stripe.String(idempotencyKey + ":" + auth.ID),
				Metadata: map[string]string{
					"bucket": s.bucket,
					"idempotencyKey": idempotencyKey + ":" + auth.ID,
				},
			},
		})
		if refund != nil && refund.ID != "" {
			s.storage.UpsertCharge(*refund.Charge)
		}
		if err == nil && refund != nil && refund.Status != "failed" {
			totalReleased += uint(releaseAmount)
		}
		if err != nil {
			lastErr = err
		}
	}
	return totalReleased, lastErr
}

// Release releases authorized funds back to the user.  It returns how much was successfully released
func (s stripeHandler) Release(idempotencyKey string, amount uint) (uint, error) {
	auths := s.storage.GetAuthorizationsFor(amount)
	return s.doRelease(auths, idempotencyKey, amount)
}

func (s stripeHandler) CaptureRelease(captureKey string, capture uint, releaseKey string, release uint) (captured uint, captureErr error, released uint, releaseErr error) {
	totalCaptured, totalReleased, captureErr := s.doCapture(captureKey, capture)
	overReleased := int(totalReleased) - int(release)
	// By capturing, we released more than we intended to, so reauthorize that amount
	if overReleased > 0 {
		reauthErr := s.Authorize(releaseKey, uint(overReleased))
		if reauthErr == nil {
			totalReleased -= uint(overReleased)
		}
		if reauthErr != nil && captureErr == nil {
			captureErr = reauthErr
		}
	} else if overReleased < 0 {
		// If we have more to release, release it now
		var additionalRelease uint
		additionalRelease, releaseErr = s.Release(releaseKey, uint(-overReleased))
		totalReleased += additionalRelease
	}
	return totalCaptured, captureErr, totalReleased, releaseErr
}

func (s stripeHandler) Charge(idempotencyKey string, amount uint) error {
	return s.doCharge(false, idempotencyKey, amount)
}

func (s stripeHandler) Refund(idempotencyKey string, amount uint) (uint, error) {
	charges := s.storage.GetChargesFor(amount)
	return s.doRelease(charges, idempotencyKey, amount)
}

func NewStripeHandler(api *client.API, cardID, currency, bucket string, storage StripeStorage) *stripeHandler {
	h := &stripeHandler{
		api,
		cardID,
		bucket,
		stripe.Currency(currency),
		storage,
	}
	return h
}

// TestNewStripeHandler should be used for testing only.  It returns a function to set the card id, which can be used
// to force stripe errors.
func TestNewStripeHandler(api *client.API, cardID, currency, bucket string, storage StripeStorage) (handler *stripeHandler, setCardId func(string)) {
	handler = NewStripeHandler(api, cardID, currency, bucket, storage)
	setCardId = func(id string) {
		handler.cardID = id
	}
	return
}