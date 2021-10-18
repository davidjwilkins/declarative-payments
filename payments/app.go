package payments

import (
	"github.com/davidjwilkins/declarative-payments/consts"
	"github.com/davidjwilkins/declarative-payments/errors"
	"github.com/davidjwilkins/declarative-payments/payments/resolver"
	"github.com/google/uuid"
	"math"
	"sync"
	"time"
)

type PartnerHandler interface {
	Deposit(idempotencyKey string, amount uint) error
	Withdraw(idempotencyKey string, amount uint) error
}

type UserHandler interface {
	Authorize(idempotencyKey string, amount uint) error
	Capture(idempotencyKey string, amount uint) error
	Release(idempotencyKey string, amount uint) error
	CaptureRelease(captureKey string, capture uint, releaseKey string, release uint) (error, error)
	Charge(idempotencyKey string, amount uint) error
	Refund(idempotencyKey string, amount uint) error
}

type ActualState struct {
	resolver.DesiredState
	Status consts.PaymentStatus
}

type handler struct {
	partner      PartnerHandler
	user         UserHandler
	currentState *ActualState
	sync.RWMutex
}

func (h handler) UserID() uuid.UUID {
	return h.currentState.UserID
}

func (h handler) PartnerID() uuid.UUID {
	return h.currentState.PartnerID
}

func (h handler) ExternalID() uuid.UUID {
	return h.currentState.ExternalID
}

func (h handler) Bucket() string {
	return h.currentState.Bucket
}

func NewHandler(currentState *ActualState, partnerHandler PartnerHandler, userHandler UserHandler) *handler {
	return &handler{
		partner:      partnerHandler,
		user:         userHandler,
		currentState: currentState,
	}
}

func (h *handler) CurrentState() ActualState {
	h.RLock()
	state := h.currentState
	h.RUnlock()
	return *state
}

func (h *handler) Run(cmds []resolver.PaymentCommand) ([]resolver.PaymentCommand, []error) {
	var wg sync.WaitGroup
	var errs []error
	var locker sync.Mutex
	wg.Add(len(cmds))

	// Some providers release the remainder when capturing.  For these, we need to know how much to release and capture
	// at the same time, so the handler can reauthorize as appropriate
	var captureRelease struct{
		capture *resolver.PaymentCommand
		captureIndex int
		release *resolver.PaymentCommand
		releaseIndex int
	}
	for i := range cmds {
		switch cmds[i].Action {
		case consts.PaymentCommandActionCapture:
			captureRelease.capture = &cmds[i]
			captureRelease.captureIndex = i
		case consts.PaymentCommandActionRelease:
			captureRelease.release = &cmds[i]
			captureRelease.releaseIndex = i
		}
	}

	handleErr := func(err error, i int) {
		if err != nil {
			locker.Lock()
			errs = append(errs, err)
			locker.Unlock()
			cmds[i].Error = err.Error()
			if errors.Is(err, errors.ErrRetryable) {
				cmds[i].Status = consts.PaymentCommandStatusError
			} else {
				cmds[i].Status = consts.PaymentCommandStatusFailed
			}
		} else {
			cmds[i].Status = consts.PaymentCommandStatusComplete
		}
	}
	for i := range cmds {
		go func(i int) {
			defer wg.Done()
			key := cmds[i].ID.String()
			cmds[i].Error = ""
			var err error
			switch cmds[i].Action {
			case consts.PaymentCommandActionAuthorize:
				err = h.user.Authorize(key, cmds[i].Amount)
				if err == nil {
					h.Lock()
					h.currentState.AuthorizedAmount += cmds[i].Amount
					h.Unlock()
				}
			case consts.PaymentCommandActionCapture:
				// Some handlers do not support incremental capturing - when you capture, the remainder is released.  For
				// these, they will need to know how much to capture and release at the same time.
				if captureRelease.release == nil {
					err = h.user.Capture(key, cmds[i].Amount)
					if err == nil {
						h.Lock()
						h.currentState.AuthorizedAmount -= cmds[i].Amount
						h.currentState.Amount += int(cmds[i].Amount)
						h.Unlock()
					}
				} else {
					captureErr, releaseErr := h.user.CaptureRelease(captureRelease.capture.ID.String(), captureRelease.capture.Amount, captureRelease.release.ID.String(), captureRelease.release.Amount)
					if captureErr == nil {
						h.Lock()
						h.currentState.AuthorizedAmount -= captureRelease.capture.Amount
						h.currentState.Amount += int(captureRelease.capture.Amount)
						h.Unlock()
					}
					if releaseErr == nil {
						h.Lock()
						h.currentState.AuthorizedAmount -= captureRelease.release.Amount
						h.Unlock()
					}
					handleErr(captureErr, captureRelease.captureIndex)
					handleErr(releaseErr, captureRelease.releaseIndex)
				}
			case consts.PaymentCommandActionRelease:
				if captureRelease.capture == nil {
					err = h.user.Release(key, cmds[i].Amount)
					if err == nil {
						h.Lock()
						h.currentState.AuthorizedAmount -= cmds[i].Amount
						h.Unlock()
					}
				}
			case consts.PaymentCommandActionCharge:
				err = h.user.Charge(key, cmds[i].Amount)
				if err == nil {
					h.Lock()
					h.currentState.Amount += int(cmds[i].Amount)
					h.Unlock()
				}
			case consts.PaymentCommandActionRefund:
				err = h.user.Refund(key, cmds[i].Amount)
				if err == nil {
					h.Lock()
					h.currentState.Amount -= int(cmds[i].Amount)
					h.Unlock()
				}
			case consts.PaymentCommandActionDeposit:
				err = h.partner.Deposit(key, cmds[i].Amount)
				if err == nil {
					h.Lock()
					h.currentState.PartnerAmount += int(cmds[i].Amount)
					h.Unlock()
				}
			case consts.PaymentCommandActionWithdraw:
				err = h.partner.Withdraw(key, cmds[i].Amount)
				if err == nil {
					h.Lock()
					h.currentState.PartnerAmount -= int(cmds[i].Amount)
					h.Unlock()
				}
			}
			cmds[i].Attempts++
			handleErr(err, i)
		}(i)
	}

	wg.Wait()
	return cmds, errs
}

func (h handler) GenerateResolution(d resolver.DesiredState) ([]resolver.PaymentCommand, error) {
	if d.Bucket != h.currentState.Bucket {
		return nil, errors.ErrDifferentBucket
	}
	if d.UserID != h.currentState.UserID {
		return nil, errors.ErrDifferentUser
	}
	if d.PartnerID != h.currentState.PartnerID {
		return nil, errors.ErrDifferentPartner
	}
	if d.Date.After(time.Now()) {
		return nil, errors.ErrDateInFuture
	}
	if d.Date.Before(h.currentState.Date) {
		return nil, errors.ErrLaterStateApplied
	}
	currentUserBalance := h.currentState.Amount
	desiredUserBalance := d.Amount
	chargeAmount := desiredUserBalance - currentUserBalance

	currentAuthorizedBalance := h.currentState.AuthorizedAmount
	desiredAuthorizedBalance := d.AuthorizedAmount
	if currentAuthorizedBalance > math.MaxInt || desiredAuthorizedBalance > math.MaxInt {
		return nil, errors.ErrUnderflow
	}
	authorizeAmount := int(desiredAuthorizedBalance) - int(currentAuthorizedBalance)
	var captureAmount int
	if chargeAmount > 0 {
		if authorizeAmount < 0 {
			// We can capture some authorized, instead of charging.
			if -authorizeAmount > chargeAmount {
				// reduce the auth-refund amount by the charge amount
				authorizeAmount += chargeAmount
				// capture the full amount of the charge
				captureAmount = chargeAmount
				// charge nothing
				chargeAmount = 0
			} else if authorizeAmount != 0 {
				// -authorizeAmount is less than the charge amount, so we need to decrease chargeAmount,
				// but authorizeAmount is negative, so just add it
				chargeAmount += authorizeAmount
				captureAmount = -authorizeAmount
				authorizeAmount = 0
			}
		}
	}

	cmds := []resolver.PaymentCommand{}

	if captureAmount > 0 {
		cmds = append(cmds, d.Capture(uint(captureAmount)))
	}

	if authorizeAmount < 0 {
		cmds = append(cmds, d.Release(uint(-authorizeAmount)))
	} else if authorizeAmount > 0 {
		cmds = append(cmds, d.Authorize(uint(authorizeAmount)))
	}
	if chargeAmount > 0 {
		cmds = append(cmds, d.Charge(uint(chargeAmount)))
	} else if chargeAmount < 0 {
		cmds = append(cmds, d.Refund(uint(-chargeAmount)))
	}

	partnerAmount := h.currentState.PartnerAmount
	desiredPartnerAmount := d.PartnerAmount
	depositAmount := desiredPartnerAmount - partnerAmount
	if depositAmount > 0 {
		cmds = append(cmds, d.Deposit(uint(depositAmount)))
	} else if depositAmount < 0 {
		cmds = append(cmds, d.Withdraw(uint(-depositAmount)))
	}
	return cmds, nil
}
