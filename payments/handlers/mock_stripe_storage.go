package handlers

import (
	"github.com/stripe/stripe-go/v72"
	"sort"
)

type mockStripeStorage struct {
	bucket     string
	Charges    map[string]stripe.Charge
}

func NewMockStripeStorage(bucket string) *mockStripeStorage {
	return &mockStripeStorage{
		bucket,
		make(map[string]stripe.Charge),
	}
}
func (m *mockStripeStorage) ListAuthorizations() []stripe.Charge {
	authorizations := []stripe.Charge{}
	for _, charge := range m.Charges {
		if !charge.Captured && !charge.Disputed && !charge.Refunded && charge.Status != "failed" {
			authorizations = append(authorizations, charge)
		}
	}
	return authorizations
}

func (m *mockStripeStorage) ListCharges() []stripe.Charge {
	charges := []stripe.Charge{}
	for _, charge := range m.Charges {
		if charge.Captured && !charge.Disputed && !charge.Refunded && charge.Status != "failed" {
			charges = append(charges, charge)
		}
	}
	return charges
}

func (m *mockStripeStorage) GetAuthorizationsFor(amount uint) []stripe.Charge {
	auths := m.ListAuthorizations()
	sort.Slice(auths, func(i, j int) bool {
		return auths[i].Created < auths[j].Created
	})
	var i int
	intAmount := int(amount)
	for i = 0; i < len(auths) && intAmount > 0; i++ {
		if auths[i].Amount > auths[i].AmountRefunded && auths[i].Paid {
			intAmount -= int(auths[i].Amount - auths[i].AmountRefunded)
		}
	}
	return auths[:i]
}

func (m *mockStripeStorage) GetChargesFor(amount uint) []stripe.Charge {
	charges := m.ListCharges()
	sort.Slice(charges, func(i, j int) bool {
		return charges[i].Created < charges[j].Created
	})
	var i int
	intAmount := int(amount)
	for i = 0; i < len(charges) && intAmount > 0; i++ {
		if charges[i].Amount > charges[i].AmountRefunded {
			intAmount -= int(charges[i].Amount - charges[i].AmountRefunded)
		}
	}
	return charges[:i]
}

func (m *mockStripeStorage) UpsertCharge(ch stripe.Charge) {
	m.Charges[ch.ID] = ch
}

func (m *mockStripeStorage) Balance() uint {
	total := uint(0)
	for _, ch := range m.Charges {
		if !ch.Disputed && !ch.Refunded && ch.Status != "failed" && ch.Captured {
			total += uint(ch.Amount - ch.AmountRefunded)
		}
	}
	return total
}

func (m *mockStripeStorage) AuthorizedBalance() uint {
	total := uint(0)
	for _, ch := range m.Charges {
		if !ch.Captured && !ch.Disputed && !ch.Refunded && ch.Status != "failed" {
			total += uint(ch.Amount - ch.AmountRefunded)
		}
	}
	return total
}