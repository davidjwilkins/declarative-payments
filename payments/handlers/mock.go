package handlers

import (
	"errors"
	"sync"
)

type userMock struct {
	lock              sync.RWMutex
	balance           int
	authorizedBalance uint
	handled           map[string]struct{}
	errors            map[string]error
}

type partnerMock struct {
	lock    sync.RWMutex
	balance int
	handled map[string]struct{}
	errors  map[string]error
}

func NewUserMock() *userMock {
	return &userMock{
		balance:           0,
		authorizedBalance: 0,
		handled:           make(map[string]struct{}),
		errors:            make(map[string]error),
	}
}

func (m *userMock) ShouldErr(key string, err error) {
	m.lock.Lock()
	defer m.lock.Unlock()
	m.errors[key] = err
}

func (m *userMock) ifNotHandled(key string, fn func()) error {
	m.lock.Lock()
	defer m.lock.Unlock()
	if err, ok := m.errors[key]; ok {
		delete(m.errors, key)
		return err
	}
	if _, ok := m.handled[key]; !ok {
		m.handled[key] = struct{}{}
		fn()
	}
	return nil
}

func (m *userMock) Balance() int {
	m.lock.RLock()
	defer m.lock.RUnlock()
	return m.balance
}

func (m *userMock) AuthorizedBalance() uint {
	m.lock.RLock()
	defer m.lock.RUnlock()
	return m.authorizedBalance
}

func (m *userMock) Authorize(idempotencyKey string, amount uint) error {
	return m.ifNotHandled(idempotencyKey, func() {
		m.authorizedBalance += amount
	})
}

func (m *userMock) Capture(idempotencyKey string, amount uint) (uint, error) {
	authorizedBalance := m.AuthorizedBalance()
	if authorizedBalance < amount {
		return 0, errors.New("cannot capture more than authorized")
	}

	err := m.ifNotHandled(idempotencyKey, func() {
		m.authorizedBalance -= amount
		m.balance += int(amount)
	})
	if err == nil {
		return amount, nil
	}
	return 0, err
}

func (m *userMock) Release(idempotencyKey string, amount uint) (uint, error) {
	authorizedBalance := m.AuthorizedBalance()
	if authorizedBalance < amount {
		return 0, errors.New("cannot release more than authorized")
	}
	err := m.ifNotHandled(idempotencyKey, func() {
		m.authorizedBalance -= amount
	})
	if err != nil {
		return 0, err
	}
	return amount, err
}

func (m *userMock) CaptureRelease(captureKey string, capture uint, releaseKey string, release uint) (uint, error, uint, error) {
	captured, captureErr := m.Capture(captureKey, capture)
	released, releaseErr := m.Release(releaseKey, release)
	return captured, captureErr, released, releaseErr
}

func (m *userMock) Charge(idempotencyKey string, amount uint) error {
	return m.ifNotHandled(idempotencyKey, func() {
		m.balance += int(amount)
	})
}

func (m *userMock) Refund(idempotencyKey string, amount uint) (uint, error) {
	err := m.ifNotHandled(idempotencyKey, func() {
		m.balance -= int(amount)
	})
	if err != nil {
		return 0, err
	}
	return amount, nil
}

func NewPartnerMock() *partnerMock {
	return &partnerMock{
		balance: 0,
		handled: make(map[string]struct{}),
		errors:  make(map[string]error),
	}
}

func (m *partnerMock) ShouldErr(key string, err error) {
	m.lock.Lock()
	defer m.lock.Unlock()
	m.errors[key] = err
}

func (m *partnerMock) ifNotHandled(key string, fn func()) error {
	m.lock.Lock()
	defer m.lock.Unlock()
	if err, ok := m.errors[key]; ok {
		delete(m.errors, key)
		return err
	}
	if _, ok := m.handled[key]; !ok {
		m.handled[key] = struct{}{}
		fn()
	}
	return nil
}

func (m *partnerMock) Balance() int {
	m.lock.RLock()
	defer m.lock.RUnlock()
	return m.balance
}

func (m *partnerMock) Deposit(idempotencyKey string, amount uint) error {
	return m.ifNotHandled(idempotencyKey, func() {
		m.balance += int(amount)
	})
}

func (m *partnerMock) Withdraw(idempotencyKey string, amount uint) error {
	return m.ifNotHandled(idempotencyKey, func() {
		m.balance -= int(amount)
	})
}
