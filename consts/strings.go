package consts

type PaymentStatus string

const (
	PaymentStatusPending  PaymentStatus = "pending"
	PaymentStatusComplete PaymentStatus = "complete"
	PaymentStatusError    PaymentStatus = "error"
)

type PaymentCommandAction string

const (
	PaymentCommandActionAuthorize      PaymentCommandAction = "authorize"
	PaymentCommandActionCapture        PaymentCommandAction = "capture"
	PaymentCommandActionRelease        PaymentCommandAction = "release"
	PaymentCommandActionCaptureRelease PaymentCommandAction = "capture-release"
	PaymentCommandActionCharge         PaymentCommandAction = "charge"
	PaymentCommandActionRefund         PaymentCommandAction = "refund"
	PaymentCommandActionDeposit        PaymentCommandAction = "deposit"
	PaymentCommandActionWithdraw       PaymentCommandAction = "withdraw"
)

type PaymentCommandStatus string

const (
	PaymentCommandStatusPending  PaymentCommandStatus = "pending"
	PaymentCommandStatusComplete PaymentCommandStatus = "complete"
	PaymentCommandStatusError    PaymentCommandStatus = "error"
	PaymentCommandStatusFailed   PaymentCommandStatus = "failed"
)
