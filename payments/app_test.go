package payments_test

import (
	"fmt"
	"github.com/davidjwilkins/declarative-payments/consts"
	"github.com/davidjwilkins/declarative-payments/errors"
	"github.com/davidjwilkins/declarative-payments/payments"
	"github.com/davidjwilkins/declarative-payments/payments/handlers"
	"github.com/davidjwilkins/declarative-payments/payments/resolver"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"math"
	"testing"
	"time"
)

type Handler interface {
	Run(cmds []resolver.PaymentCommand) ([]resolver.PaymentCommand, []error)
	GenerateResolution(d resolver.DesiredState) ([]resolver.PaymentCommand, error)
	UserID() uuid.UUID
	PartnerID() uuid.UUID
	ExternalID() uuid.UUID
	Bucket() string
	CurrentState() payments.ActualState
}

func withErrorsMockHandler(fns ...func(as *payments.ActualState)) (Handler, *payments.ActualState, resolver.DesiredState, func(string, error), func(string, error)) {
	userHandler := handlers.NewUserMock()
	partnerHandler := handlers.NewPartnerMock()
	as := payments.ActualState{
		DesiredState: resolver.DesiredState{
			ID:               uuid.New(),
			ExternalID:       uuid.New(),
			UserID:           uuid.New(),
			PartnerID:        uuid.New(),
			Date:             time.Now().Add(-10 * time.Minute),
			Bucket:           "test",
			Amount:           0,
			AuthorizedAmount: 0,
			PartnerAmount:    0,
		},
		Status: consts.PaymentStatusComplete,
	}
	for _, fn := range fns {
		fn(&as)
	}
	handler := payments.NewHandler(
		&as,
		partnerHandler,
		userHandler,
	)
	state := resolver.DesiredState{
		ID:         uuid.New(),
		ExternalID: handler.ExternalID(),
		UserID:     handler.UserID(),
		PartnerID:  handler.PartnerID(),
		Date:       time.Now(),
		Bucket:     handler.Bucket(),
	}
	return handler, &as, state, userHandler.ShouldErr, partnerHandler.ShouldErr
}

func mockHandler(fns ...func(as *payments.ActualState)) (Handler, *payments.ActualState, resolver.DesiredState) {
	handler, as, state, _, _ := withErrorsMockHandler(fns...)
	return handler, as, state
}

func TestNewHandler(t *testing.T) {
	handler, _, _ := mockHandler()
	assert.NotNil(t, handler, "Can create handler")
}

func TestHandler_Run(t *testing.T) {
	wasSuccessful := func(action consts.PaymentCommandAction, cmds []resolver.PaymentCommand) {
		assert.Equal(t, 1, len(cmds))
		assert.Equal(t, action, cmds[0].Action)
		assert.Equal(t, uint(1000), cmds[0].Amount)
		assert.Equal(t, uint(1), cmds[0].Attempts)
		assert.Equal(t, consts.PaymentCommandStatusComplete, cmds[0].Status)
		assert.Empty(t, cmds[0].Error)
	}
	t.Run("Charge", func(t *testing.T) {
		handler, state, ds := mockHandler()
		cmd := ds.Charge(1000)
		cmds, errs := handler.Run([]resolver.PaymentCommand{
			cmd,
		})
		assert.Equal(t, 0, len(errs))
		assert.Equal(t, ds.ID, cmds[0].DesiredStateID)
		assert.Equal(t, cmd.ID, cmds[0].ID)
		wasSuccessful(consts.PaymentCommandActionCharge, cmds)
		assert.Equal(t, 1000, state.Amount)
	})
	t.Run("Authorize", func(t *testing.T) {
		handler, state, ds := mockHandler()
		cmd := ds.Authorize(1000)
		cmds, errs := handler.Run([]resolver.PaymentCommand{
			cmd,
		})
		assert.Equal(t, 0, len(errs))
		assert.Equal(t, ds.ID, cmds[0].DesiredStateID)
		assert.Equal(t, cmd.ID, cmds[0].ID)
		wasSuccessful(consts.PaymentCommandActionAuthorize, cmds)
		assert.Equal(t, uint(1000), state.AuthorizedAmount)
	})
	t.Run("Capture", func(t *testing.T) {
		handler, state, ds := mockHandler()
		cmd := ds.Capture(1000)
		cmds, errs := handler.Run([]resolver.PaymentCommand{
			cmd,
		})
		assert.Equal(t, 1, len(errs))
		assert.Equal(t, errs[0].Error(), "cannot capture more than authorized")
		assert.Equal(t, consts.PaymentCommandStatusFailed, cmds[0].Status, "cannot capture more than authorized")
		assert.Equal(t, ds.ID, cmds[0].DesiredStateID)
		assert.Equal(t, cmd.ID, cmds[0].ID)
		assert.Equal(t, 0, state.Amount)
		handler.Run([]resolver.PaymentCommand{
			ds.Authorize(1000),
		})
		assert.Equal(t, uint(1000), state.AuthorizedAmount)
		cmds, errs = handler.Run([]resolver.PaymentCommand{
			cmd,
		})
		assert.Equal(t, 0, len(errs))
		assert.Equal(t, ds.ID, cmds[0].DesiredStateID)
		assert.Equal(t, cmd.ID, cmds[0].ID)
		wasSuccessful(consts.PaymentCommandActionCapture, cmds)
		assert.Equal(t, uint(0), state.AuthorizedAmount)
		assert.Equal(t, 1000, state.Amount)
	})
	t.Run("Release", func(t *testing.T) {
		handler, state, ds := mockHandler()
		cmd := ds.Release(1000)
		cmds, errs := handler.Run([]resolver.PaymentCommand{
			cmd,
		})
		assert.Equal(t, 1, len(errs))
		assert.Equal(t, errs[0].Error(), "cannot release more than authorized")
		assert.Equal(t, consts.PaymentCommandStatusFailed, cmds[0].Status, "cannot release more than authorized")
		assert.Equal(t, ds.ID, cmds[0].DesiredStateID)
		assert.Equal(t, cmd.ID, cmds[0].ID)
		assert.Equal(t, 0, state.Amount)
		handler.Run([]resolver.PaymentCommand{
			ds.Authorize(1000),
		})
		assert.Equal(t, uint(1000), state.AuthorizedAmount)
		cmds, errs = handler.Run([]resolver.PaymentCommand{
			cmd,
		})
		assert.Equal(t, 0, len(errs))
		assert.Equal(t, ds.ID, cmds[0].DesiredStateID)
		assert.Equal(t, cmd.ID, cmds[0].ID)
		wasSuccessful(consts.PaymentCommandActionRelease, cmds)
		assert.Equal(t, uint(0), state.AuthorizedAmount)
		assert.Equal(t, 0, state.Amount)
	})
	t.Run("Refund", func(t *testing.T) {
		handler, state, ds := mockHandler(func(as *payments.ActualState) {
			as.Amount = 1000
		})
		cmd := ds.Refund(1000)
		cmds, errs := handler.Run([]resolver.PaymentCommand{
			cmd,
		})
		assert.Equal(t, 0, len(errs))
		assert.Equal(t, ds.ID, cmds[0].DesiredStateID)
		assert.Equal(t, cmd.ID, cmds[0].ID)
		wasSuccessful(consts.PaymentCommandActionRefund, cmds)
		assert.Equal(t, 0, state.Amount)
	})
	t.Run("Withdraw", func(t *testing.T) {
		handler, state, ds := mockHandler(func(as *payments.ActualState) {
			as.PartnerAmount = 1000
		})
		cmd := ds.Withdraw(1000)
		cmds, errs := handler.Run([]resolver.PaymentCommand{
			cmd,
		})
		assert.Equal(t, 0, len(errs))
		assert.Equal(t, ds.ID, cmds[0].DesiredStateID)
		assert.Equal(t, cmd.ID, cmds[0].ID)
		wasSuccessful(consts.PaymentCommandActionWithdraw, cmds)
		assert.Equal(t, 0, state.PartnerAmount)
	})
	t.Run("Deposit", func(t *testing.T) {
		handler, state, ds := mockHandler()
		cmd := ds.Deposit(1000)
		cmds, errs := handler.Run([]resolver.PaymentCommand{
			cmd,
		})
		assert.Equal(t, 0, len(errs))
		assert.Equal(t, ds.ID, cmds[0].DesiredStateID)
		assert.Equal(t, cmd.ID, cmds[0].ID)
		wasSuccessful(consts.PaymentCommandActionDeposit, cmds)
		assert.Equal(t, 1000, state.PartnerAmount)
	})
	t.Run("Capture + Release", func(t *testing.T) {
		handler, state, ds := mockHandler()
		handler.Run([]resolver.PaymentCommand{
			ds.Authorize(2400),
		})
		originalCmds := []resolver.PaymentCommand{
			ds.Capture(1000),
			ds.Release(1000),
		}
		cmds, errs := handler.Run(originalCmds)
		assert.Equal(t, 0, len(errs))
		assert.Equal(t, ds.ID, cmds[0].DesiredStateID)
		assert.Equal(t, ds.ID, cmds[1].DesiredStateID)
		assert.Equal(t, originalCmds[0].ID, cmds[0].ID)
		assert.Equal(t, originalCmds[1].ID, cmds[1].ID)
		wasSuccessful(consts.PaymentCommandActionCapture, cmds[:1])
		wasSuccessful(consts.PaymentCommandActionRelease, cmds[1:])
		assert.Equal(t, uint(400), state.AuthorizedAmount)
		assert.Equal(t, 1000, state.Amount)
	})

	t.Run("Retryable error is not failure", func(t *testing.T) {
		handler, state, ds, userErr, _ := withErrorsMockHandler()
		cmd := ds.Charge(1000)
		userErr(cmd.ID.String(), fmt.Errorf("Internal Server Error - %w", errors.ErrRetryable))
		cmds, errs := handler.Run([]resolver.PaymentCommand{
			cmd,
		})
		assert.Equal(t, 1, len(errs))
		assert.Equal(t, 1, len(cmds))
		assert.Equal(t, ds.ID, cmds[0].DesiredStateID)
		assert.Equal(t, cmd.ID, cmds[0].ID)
		assert.Equal(t, 0, state.Amount)
		assert.Equal(t, consts.PaymentCommandStatusError, cmds[0].Status)
		assert.Equal(t, uint(1), cmds[0].Attempts)
		userErr(cmd.ID.String(), fmt.Errorf("Internal Server Error - %w", errors.ErrRetryable))
		cmds, errs = handler.Run([]resolver.PaymentCommand{
			cmds[0],
		})
		assert.Equal(t, 1, len(errs))
		assert.Equal(t, 1, len(cmds))
		assert.Equal(t, consts.PaymentCommandStatusError, cmds[0].Status)
		assert.Equal(t, uint(2), cmds[0].Attempts)
		assert.Equal(t, 0, state.Amount)
		cmds, errs = handler.Run([]resolver.PaymentCommand{
			cmds[0],
		})
		assert.Equal(t, 0, len(errs))
		assert.Equal(t, 1, len(cmds))
		assert.Equal(t, consts.PaymentCommandStatusComplete, cmds[0].Status)
		assert.Equal(t, uint(3), cmds[0].Attempts)
		assert.Equal(t, 1000, state.Amount)
	})
}

func TestHandler_GenerateResolution(t *testing.T) {
	t.Run("Charge", func(t *testing.T) {
		handler, _, ds := mockHandler()
		ds.Amount = 1000
		cmds, err := handler.GenerateResolution(ds)
		assert.NoError(t, err)
		assert.Equal(t, 1, len(cmds))
		assert.Equal(t, uint(1000), cmds[0].Amount)
		assert.Equal(t, consts.PaymentCommandStatusPending, cmds[0].Status)
		assert.Empty(t, cmds[0].Error)
		assert.Equal(t, consts.PaymentCommandActionCharge, cmds[0].Action)
		assert.Equal(t, uint(0), cmds[0].Attempts)
		assert.Equal(t, ds.ID, cmds[0].DesiredStateID)
	})
	t.Run("Authorize", func(t *testing.T) {
		handler, _, ds := mockHandler()
		ds.AuthorizedAmount = 1000
		cmds, err := handler.GenerateResolution(ds)
		assert.NoError(t, err)
		assert.Equal(t, 1, len(cmds))
		assert.Equal(t, uint(1000), cmds[0].Amount)
		assert.Equal(t, consts.PaymentCommandStatusPending, cmds[0].Status)
		assert.Empty(t, cmds[0].Error)
		assert.Equal(t, consts.PaymentCommandActionAuthorize, cmds[0].Action)
		assert.Equal(t, uint(0), cmds[0].Attempts)
		assert.Equal(t, ds.ID, cmds[0].DesiredStateID)
	})
	t.Run("Full Capture", func(t *testing.T) {
		handler, _, ds := mockHandler(func(as *payments.ActualState) {
			as.AuthorizedAmount = 1000
		})
		ds.Amount = 1000
		cmds, err := handler.GenerateResolution(ds)
		assert.NoError(t, err)
		assert.Equal(t, 1, len(cmds))
		assert.Equal(t, uint(1000), cmds[0].Amount)
		assert.Equal(t, consts.PaymentCommandStatusPending, cmds[0].Status)
		assert.Empty(t, cmds[0].Error)
		assert.Equal(t, consts.PaymentCommandActionCapture, cmds[0].Action)
		assert.Equal(t, uint(0), cmds[0].Attempts)
		assert.Equal(t, ds.ID, cmds[0].DesiredStateID)
	})
	t.Run("Partial Capture", func(t *testing.T) {
		handler, _, ds := mockHandler(func(as *payments.ActualState) {
			as.AuthorizedAmount = 1000
		})
		ds.Amount = 500
		ds.AuthorizedAmount = 500
		cmds, err := handler.GenerateResolution(ds)
		assert.NoError(t, err)
		assert.Equal(t, 1, len(cmds))
		assert.Equal(t, uint(500), cmds[0].Amount)
		assert.Equal(t, consts.PaymentCommandStatusPending, cmds[0].Status)
		assert.Empty(t, cmds[0].Error)
		assert.Equal(t, consts.PaymentCommandActionCapture, cmds[0].Action)
		assert.Equal(t, uint(0), cmds[0].Attempts)
		assert.Equal(t, ds.ID, cmds[0].DesiredStateID)
	})
	t.Run("Partial Capture has release", func(t *testing.T) {
		handler, _, ds := mockHandler(func(as *payments.ActualState) {
			as.AuthorizedAmount = 1000
		})
		ds.Amount = 500
		ds.AuthorizedAmount = 0
		cmds, err := handler.GenerateResolution(ds)
		assert.NoError(t, err)
		assert.Equal(t, 2, len(cmds))
		assert.Equal(t, uint(500), cmds[0].Amount)
		assert.Equal(t, consts.PaymentCommandStatusPending, cmds[0].Status)
		assert.Empty(t, cmds[0].Error)
		assert.Equal(t, consts.PaymentCommandActionCapture, cmds[0].Action)
		assert.Equal(t, uint(0), cmds[0].Attempts)
		assert.Equal(t, ds.ID, cmds[0].DesiredStateID)
		assert.Equal(t, uint(500), cmds[1].Amount)
		assert.Equal(t, consts.PaymentCommandStatusPending, cmds[1].Status)
		assert.Empty(t, cmds[1].Error)
		assert.Equal(t, consts.PaymentCommandActionRelease, cmds[1].Action)
		assert.Equal(t, uint(0), cmds[1].Attempts)
		assert.Equal(t, ds.ID, cmds[1].DesiredStateID)
	})
	t.Run("Partial Capture does charge if auth amount is unchanged", func(t *testing.T) {
		handler, _, ds := mockHandler(func(as *payments.ActualState) {
			as.AuthorizedAmount = 1000
		})
		ds.Amount = 100
		ds.AuthorizedAmount = 1000
		cmds, err := handler.GenerateResolution(ds)
		assert.NoError(t, err)
		assert.Equal(t, 1, len(cmds))
		assert.Equal(t, uint(100), cmds[0].Amount)
		assert.Equal(t, consts.PaymentCommandStatusPending, cmds[0].Status)
		assert.Empty(t, cmds[0].Error)
		assert.Equal(t, consts.PaymentCommandActionCharge, cmds[0].Action)
		assert.Equal(t, uint(0), cmds[0].Attempts)
		assert.Equal(t, ds.ID, cmds[0].DesiredStateID)
	})
	t.Run("Can refund and authorize", func(t *testing.T) {
		handler, _, ds := mockHandler(func(as *payments.ActualState) {
			as.AuthorizedAmount = 1000
			as.Amount = 1000
		})
		ds.Amount = 0
		ds.AuthorizedAmount = 2000
		cmds, err := handler.GenerateResolution(ds)
		assert.NoError(t, err)
		assert.Equal(t, 2, len(cmds))
		assert.Equal(t, uint(1000), cmds[0].Amount)
		assert.Equal(t, consts.PaymentCommandStatusPending, cmds[0].Status)
		assert.Empty(t, cmds[0].Error)
		assert.Equal(t, consts.PaymentCommandActionAuthorize, cmds[0].Action)
		assert.Equal(t, uint(0), cmds[0].Attempts)
		assert.Equal(t, ds.ID, cmds[0].DesiredStateID)
		assert.Equal(t, uint(1000), cmds[1].Amount)
		assert.Equal(t, consts.PaymentCommandStatusPending, cmds[1].Status)
		assert.Empty(t, cmds[1].Error)
		assert.Equal(t, consts.PaymentCommandActionRefund, cmds[1].Action)
		assert.Equal(t, uint(0), cmds[1].Attempts)
		assert.Equal(t, ds.ID, cmds[1].DesiredStateID)
	})
	t.Run("Release", func(t *testing.T) {
		handler, _, ds := mockHandler(func(as *payments.ActualState) {
			as.AuthorizedAmount = 1000
		})
		ds.AuthorizedAmount = 0
		cmds, err := handler.GenerateResolution(ds)
		assert.NoError(t, err)
		assert.Equal(t, 1, len(cmds))
		assert.Equal(t, uint(1000), cmds[0].Amount)
		assert.Equal(t, consts.PaymentCommandStatusPending, cmds[0].Status)
		assert.Empty(t, cmds[0].Error)
		assert.Equal(t, consts.PaymentCommandActionRelease, cmds[0].Action)
		assert.Equal(t, uint(0), cmds[0].Attempts)
		assert.Equal(t, ds.ID, cmds[0].DesiredStateID)
	})
	t.Run("Underflowing uints detected", func(t *testing.T) {
		handler, _, ds := mockHandler(func(as *payments.ActualState) {
			as.AuthorizedAmount = 0
		})
		underflow := uint(math.MaxInt) + 1 // if a uint is greater than the max int, it almost certainly underflowed
		ds.AuthorizedAmount = underflow
		cmds, err := handler.GenerateResolution(ds)
		assert.Error(t, err)
		assert.Equal(t, 0, len(cmds))
	})
	t.Run("Refund", func(t *testing.T) {
		handler, _, ds := mockHandler(func(as *payments.ActualState) {
			as.Amount = 1000
		})
		ds.Amount = 400
		cmds, err := handler.GenerateResolution(ds)
		assert.NoError(t, err)
		assert.Equal(t, 1, len(cmds))
		assert.Equal(t, uint(600), cmds[0].Amount)
		assert.Equal(t, consts.PaymentCommandStatusPending, cmds[0].Status)
		assert.Empty(t, cmds[0].Error)
		assert.Equal(t, consts.PaymentCommandActionRefund, cmds[0].Action)
		assert.Equal(t, uint(0), cmds[0].Attempts)
		assert.Equal(t, ds.ID, cmds[0].DesiredStateID)
	})
	t.Run("Refund To Negative", func(t *testing.T) {
		handler, _, ds := mockHandler(func(as *payments.ActualState) {
			as.Amount = 0
		})
		ds.Amount = -600
		cmds, err := handler.GenerateResolution(ds)
		assert.NoError(t, err)
		assert.Equal(t, 1, len(cmds))
		assert.Equal(t, uint(600), cmds[0].Amount)
		assert.Equal(t, consts.PaymentCommandStatusPending, cmds[0].Status)
		assert.Empty(t, cmds[0].Error)
		assert.Equal(t, consts.PaymentCommandActionRefund, cmds[0].Action)
		assert.Equal(t, uint(0), cmds[0].Attempts)
		assert.Equal(t, ds.ID, cmds[0].DesiredStateID)
	})
	t.Run("Deposit", func(t *testing.T) {
		handler, _, ds := mockHandler(func(as *payments.ActualState) {
			as.PartnerAmount = 0
		})
		ds.PartnerAmount = 1000
		cmds, err := handler.GenerateResolution(ds)
		assert.NoError(t, err)
		assert.Equal(t, 1, len(cmds))
		assert.Equal(t, uint(1000), cmds[0].Amount)
		assert.Equal(t, consts.PaymentCommandStatusPending, cmds[0].Status)
		assert.Empty(t, cmds[0].Error)
		assert.Equal(t, consts.PaymentCommandActionDeposit, cmds[0].Action)
		assert.Equal(t, uint(0), cmds[0].Attempts)
		assert.Equal(t, ds.ID, cmds[0].DesiredStateID)
	})
	t.Run("Deposit From Negative", func(t *testing.T) {
		handler, _, ds := mockHandler(func(as *payments.ActualState) {
			as.PartnerAmount = -1000
		})
		ds.PartnerAmount = 0
		cmds, err := handler.GenerateResolution(ds)
		assert.NoError(t, err)
		assert.Equal(t, 1, len(cmds))
		assert.Equal(t, uint(1000), cmds[0].Amount)
		assert.Equal(t, consts.PaymentCommandStatusPending, cmds[0].Status)
		assert.Empty(t, cmds[0].Error)
		assert.Equal(t, consts.PaymentCommandActionDeposit, cmds[0].Action)
		assert.Equal(t, uint(0), cmds[0].Attempts)
		assert.Equal(t, ds.ID, cmds[0].DesiredStateID)
	})
	t.Run("Withdraw", func(t *testing.T) {
		handler, _, ds := mockHandler(func(as *payments.ActualState) {
			as.PartnerAmount = 1000
		})
		ds.PartnerAmount = 0
		cmds, err := handler.GenerateResolution(ds)
		assert.NoError(t, err)
		assert.Equal(t, 1, len(cmds))
		assert.Equal(t, uint(1000), cmds[0].Amount)
		assert.Equal(t, consts.PaymentCommandStatusPending, cmds[0].Status)
		assert.Empty(t, cmds[0].Error)
		assert.Equal(t, consts.PaymentCommandActionWithdraw, cmds[0].Action)
		assert.Equal(t, uint(0), cmds[0].Attempts)
		assert.Equal(t, ds.ID, cmds[0].DesiredStateID)
	})
	t.Run("Withdraw To Negative", func(t *testing.T) {
		handler, _, ds := mockHandler(func(as *payments.ActualState) {
			as.PartnerAmount = 0
		})
		ds.PartnerAmount = -1000
		cmds, err := handler.GenerateResolution(ds)
		assert.NoError(t, err)
		assert.Equal(t, 1, len(cmds))
		assert.Equal(t, uint(1000), cmds[0].Amount)
		assert.Equal(t, consts.PaymentCommandStatusPending, cmds[0].Status)
		assert.Empty(t, cmds[0].Error)
		assert.Equal(t, consts.PaymentCommandActionWithdraw, cmds[0].Action)
		assert.Equal(t, uint(0), cmds[0].Attempts)
		assert.Equal(t, ds.ID, cmds[0].DesiredStateID)
	})

	t.Run("Can handle no-op resolution", func(t *testing.T) {
		handler, _, ds := mockHandler()
		cmds, err := handler.GenerateResolution(ds)
		assert.Equal(t, 0, len(cmds))
		assert.NoError(t, err)
	})

	t.Run("Desired state must match handler", func(t *testing.T) {
		t.Run("Bucket must match", func(t *testing.T) {
			handler, _, ds := mockHandler()
			ds.Bucket = "fail"
			_, err := handler.GenerateResolution(ds)
			assert.Error(t, err)
		})
		t.Run("User must match", func(t *testing.T) {
			handler, _, ds := mockHandler()
			ds.UserID = uuid.New()
			_, err := handler.GenerateResolution(ds)
			assert.Error(t, err)
		})
		t.Run("Partner must match", func(t *testing.T) {
			handler, _, ds := mockHandler()
			ds.PartnerID = uuid.New()
			_, err := handler.GenerateResolution(ds)
			assert.Error(t, err)
		})
	})

	t.Run("Desired state must not be in future", func(t *testing.T) {
		handler, _, ds := mockHandler()
		ds.Date = time.Now().Add(time.Minute)
		_, err := handler.GenerateResolution(ds)
		assert.Error(t, err)
	})

	t.Run("Desired state must be more recent than current state", func(t *testing.T) {
		handler, as, ds := mockHandler()
		ds.Date = as.Date.Add(-time.Minute)
		_, err := handler.GenerateResolution(ds)
		assert.Error(t, err)
	})

	t.Run("CurrentState is a copy", func(t *testing.T) {
		handler, as, _ := mockHandler()
		as.Amount = 100
		currentState := handler.CurrentState()
		currentState.Amount = 200
		assert.NotEqual(t, as.Amount, currentState.Amount)
		currentState = handler.CurrentState()
		assert.Equal(t, as.Amount, currentState.Amount)
	})
}
