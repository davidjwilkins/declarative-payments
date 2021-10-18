package resolver_test

import (
	"github.com/davidjwilkins/declarative-payments/consts"
	"github.com/davidjwilkins/declarative-payments/payments/resolver"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"testing"
	"time"
)

func TestDesiredState(t *testing.T) {
	d := resolver.DesiredState{
		ID:         uuid.New(),
		ExternalID: uuid.New(),
		UserID:     uuid.New(),
		Date:       time.Now(),
		Bucket:     "test",
	}
	var testCases = map[consts.PaymentCommandAction]func(amount uint) resolver.PaymentCommand{
		consts.PaymentCommandActionAuthorize: d.Authorize,
		consts.PaymentCommandActionCapture:   d.Capture,
		consts.PaymentCommandActionRelease:   d.Release,
		consts.PaymentCommandActionCharge:    d.Charge,
		consts.PaymentCommandActionRefund:    d.Refund,
		consts.PaymentCommandActionDeposit:   d.Deposit,
		consts.PaymentCommandActionWithdraw:  d.Withdraw,
	}

	for action, test := range testCases {
		t.Run(string(action), func(t *testing.T) {
			cmd := test(100)
			assert.Equal(t, cmd.Action, action)
			assert.Equal(t, cmd.Amount, uint(100))
			assert.Equal(t, cmd.DesiredStateID, d.ID)
			assert.Equal(t, cmd.Status, consts.PaymentCommandStatusPending)
			assert.Equal(t, cmd.Error, "")
			assert.Equal(t, cmd.Attempts, uint(0))
		})
	}
}
