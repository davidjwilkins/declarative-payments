package resolver

import (
	"github.com/google/uuid"
	"github.com/davidjwilkins/declarative-payments/consts"
	"time"
)

type DesiredState struct {
	ID               uuid.UUID
	ExternalID       uuid.UUID
	UserID           uuid.UUID
	PartnerID        uuid.UUID
	Date             time.Time
	Bucket           string
	Amount           int
	AuthorizedAmount uint
	PartnerAmount    int
}

type PaymentCommand struct {
	ID             uuid.UUID
	DesiredStateID uuid.UUID
	Action         consts.PaymentCommandAction
	Amount         uint
	Attempts       uint
	Status         consts.PaymentCommandStatus
	Error          string
}

func (d DesiredState) generateCommand(cmd consts.PaymentCommandAction, amount uint) PaymentCommand {
	return PaymentCommand{
		ID:             uuid.New(),
		DesiredStateID: d.ID,
		Action:         cmd,
		Amount:         amount,
		Attempts:       0,
		Status:         consts.PaymentCommandStatusPending,
	}
}

func (d DesiredState) Authorize(amount uint) PaymentCommand {
	return d.generateCommand(consts.PaymentCommandActionAuthorize, amount)
}

func (d DesiredState) Capture(amount uint) PaymentCommand {
	return d.generateCommand(consts.PaymentCommandActionCapture, amount)
}

func (d DesiredState) Release(amount uint) PaymentCommand {
	return d.generateCommand(consts.PaymentCommandActionRelease, amount)
}

func (d DesiredState) Charge(amount uint) PaymentCommand {
	return d.generateCommand(consts.PaymentCommandActionCharge, amount)
}

func (d DesiredState) Refund(amount uint) PaymentCommand {
	return d.generateCommand(consts.PaymentCommandActionRefund, amount)
}

func (d DesiredState) Deposit(amount uint) PaymentCommand {
	return d.generateCommand(consts.PaymentCommandActionDeposit, amount)
}

func (d DesiredState) Withdraw(amount uint) PaymentCommand {
	return d.generateCommand(consts.PaymentCommandActionWithdraw, amount)
}
