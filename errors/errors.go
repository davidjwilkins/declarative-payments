package errors

import "errors"

var ErrUnderflow = errors.New("underflow detected")
var ErrDifferentBucket = errors.New("cannot resolve payment states for different buckets")
var ErrDifferentUser = errors.New("cannot resolve payment states for different users")
var ErrDifferentPartner = errors.New("cannot resolve payment states for different partners")
var ErrDateInFuture = errors.New("date of desired state is in the future")
var ErrLaterStateApplied = errors.New("desired state date is not the most current")
var ErrRetryable = errors.New("retryable")
var ErrChargeFailed = errors.New("charge failed")
var Is = errors.Is
