package service

import (
	"strings"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/payment"
)

func paymentProviderConfigCurrency(providerKey string, cfg map[string]string) string {
	switch strings.TrimSpace(providerKey) {
	case payment.TypeInfini:
		return payment.InfiniSettlementCurrency(cfg["currency"])
	case payment.TypeStripe, payment.TypeAirwallex:
		currency, err := payment.NormalizePaymentCurrency(cfg["currency"])
		if err == nil {
			return currency
		}
	}
	return payment.DefaultPaymentCurrency
}

// resolveOrderSettlementCurrency is the only currency QuoteOrder and CreateOrder
// use for pay amount, FX, and the order snapshot. After an instance is selected,
// both paths read the provider config. Before selection, Infini still settles in
// USD so leftover CNY/empty instance labels cannot leak into the quote.
func resolveOrderSettlementCurrency(paymentType, methodCurrency string, sel *payment.InstanceSelection) string {
	if sel != nil {
		return paymentProviderConfigCurrency(sel.ProviderKey, sel.Config)
	}
	providerKey := NormalizeVisibleMethod(paymentType)
	if providerKey == "" {
		providerKey = strings.TrimSpace(paymentType)
	}
	if providerKey == payment.TypeInfini {
		return payment.InfiniSettlementCurrency(methodCurrency)
	}
	if strings.TrimSpace(methodCurrency) != "" {
		return methodCurrency
	}
	return payment.DefaultPaymentCurrency
}

func PaymentOrderCurrency(order *dbent.PaymentOrder) string {
	if snapshot := psOrderProviderSnapshot(order); snapshot != nil {
		if currency, err := payment.CanonicalAmountCurrency(snapshot.Currency); err == nil {
			return currency
		}
	}
	return payment.DefaultPaymentCurrency
}
