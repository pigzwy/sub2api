package service

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/payment"
	"github.com/shopspring/decimal"
)

type paymentOrderProviderSnapshot struct {
	SchemaVersion      int
	ProviderInstanceID string
	ProviderKey        string
	PaymentMode        string
	MerchantAppID      string
	MerchantID         string
	Currency           string
	PackageAmount      string
	CreditAmount       string
	PayAmount          string
	FeeRate            string
	FxRate             string
	FxConverted        bool
	PaymentType        string
	OrderType          string
}

type paymentOrderFinancialSnapshot struct {
	PackageAmount string
	CreditAmount  string
	PayAmount     string
	FeeRate       string
	FxRate        string
	FxConverted   bool
	PaymentType   string
	OrderType     string
	Currency      string
}

func psOrderProviderSnapshot(order *dbent.PaymentOrder) *paymentOrderProviderSnapshot {
	if order == nil || len(order.ProviderSnapshot) == 0 {
		return nil
	}

	snapshot := &paymentOrderProviderSnapshot{
		SchemaVersion:      psSnapshotIntValue(order.ProviderSnapshot["schema_version"]),
		ProviderInstanceID: psSnapshotStringValue(order.ProviderSnapshot["provider_instance_id"]),
		ProviderKey:        psSnapshotStringValue(order.ProviderSnapshot["provider_key"]),
		PaymentMode:        psSnapshotStringValue(order.ProviderSnapshot["payment_mode"]),
		MerchantAppID:      psSnapshotStringValue(order.ProviderSnapshot["merchant_app_id"]),
		MerchantID:         psSnapshotStringValue(order.ProviderSnapshot["merchant_id"]),
		Currency:           psSnapshotStringValue(order.ProviderSnapshot["currency"]),
		PackageAmount:      psSnapshotStringValue(order.ProviderSnapshot["package_amount"]),
		CreditAmount:       psSnapshotStringValue(order.ProviderSnapshot["credit_amount"]),
		PayAmount:          psSnapshotStringValue(order.ProviderSnapshot["pay_amount"]),
		FeeRate:            psSnapshotStringValue(order.ProviderSnapshot["fee_rate"]),
		FxRate:             psSnapshotStringValue(order.ProviderSnapshot["fx_rate"]),
		FxConverted:        psSnapshotBoolValue(order.ProviderSnapshot["fx_converted"]),
		PaymentType:        psSnapshotStringValue(order.ProviderSnapshot["payment_type"]),
		OrderType:          psSnapshotStringValue(order.ProviderSnapshot["order_type"]),
	}
	if snapshot.SchemaVersion == 0 &&
		snapshot.ProviderInstanceID == "" &&
		snapshot.ProviderKey == "" &&
		snapshot.PaymentMode == "" &&
		snapshot.MerchantAppID == "" &&
		snapshot.MerchantID == "" &&
		snapshot.Currency == "" &&
		snapshot.PayAmount == "" {
		return nil
	}
	return snapshot
}

func psSnapshotStringValue(value any) string {
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed)
	default:
		return ""
	}
}

func psSnapshotBoolValue(value any) bool {
	switch typed := value.(type) {
	case bool:
		return typed
	case string:
		return strings.EqualFold(strings.TrimSpace(typed), "true")
	default:
		return false
	}
}

func newPaymentOrderFinancialSnapshot(
	req CreateOrderRequest,
	cfg *PaymentConfig,
	sel *payment.InstanceSelection,
	orderAmount, limitAmount, feeRate, payAmount float64,
) paymentOrderFinancialSnapshot {
	currency := payment.DefaultPaymentCurrency
	if sel != nil {
		currency = paymentProviderConfigCurrency(sel.ProviderKey, sel.Config)
	}
	fxRate := 0.0
	if cfg != nil {
		fxRate = normalizeSubscriptionUSDToCNYRate(cfg.SubscriptionUSDToCNYRate)
	}
	return paymentOrderFinancialSnapshot{
		PackageAmount: decimalAmountString(limitAmount, 2),
		CreditAmount:  decimalAmountString(orderAmount, 2),
		PayAmount:     formatPaymentAmountExact(payAmount, currency),
		FeeRate:       decimalAmountString(feeRate, 4),
		FxRate:        decimalAmountString(fxRate, 4),
		FxConverted:   quoteUsesFX(req.OrderType, req.PaymentType, currency, fxRate),
		PaymentType:   strings.TrimSpace(req.PaymentType),
		OrderType:     strings.TrimSpace(req.OrderType),
		Currency:      currency,
	}
}

func attachPaymentOrderFinancialSnapshot(snapshot map[string]any, fin paymentOrderFinancialSnapshot) map[string]any {
	if snapshot == nil {
		snapshot = map[string]any{}
	}
	snapshot["schema_version"] = 3
	snapshot["package_amount"] = fin.PackageAmount
	snapshot["credit_amount"] = fin.CreditAmount
	snapshot["pay_amount"] = fin.PayAmount
	snapshot["fee_rate"] = fin.FeeRate
	snapshot["fx_rate"] = fin.FxRate
	snapshot["fx_converted"] = fin.FxConverted
	if fin.PaymentType != "" {
		snapshot["payment_type"] = fin.PaymentType
	}
	if fin.OrderType != "" {
		snapshot["order_type"] = fin.OrderType
	}
	if fin.Currency != "" {
		snapshot["currency"] = fin.Currency
	}
	return snapshot
}

func decimalAmountString(amount float64, digits int32) string {
	return decimal.NewFromFloat(amount).Round(digits).StringFixed(digits)
}

func validatePaymentOrderFinancialSnapshot(order *dbent.PaymentOrder, snapshot *paymentOrderProviderSnapshot) error {
	if order == nil || snapshot == nil {
		return nil
	}
	if snapshot.PaymentType != "" && strings.TrimSpace(order.PaymentType) != "" &&
		!strings.EqualFold(snapshot.PaymentType, strings.TrimSpace(order.PaymentType)) {
		return fmt.Errorf("payment type snapshot mismatch: expected %s, got %s", snapshot.PaymentType, order.PaymentType)
	}
	if snapshot.OrderType != "" && strings.TrimSpace(order.OrderType) != "" &&
		!strings.EqualFold(snapshot.OrderType, strings.TrimSpace(order.OrderType)) {
		return fmt.Errorf("order type snapshot mismatch: expected %s, got %s", snapshot.OrderType, order.OrderType)
	}
	if snapshot.CreditAmount != "" {
		if err := paymentAmountsMatch(snapshot.CreditAmount, 0, formatPaymentAmountExact(order.Amount, "USD"), order.Amount, "USD"); err != nil {
			return fmt.Errorf("credit snapshot mismatch: %w", err)
		}
	}
	if snapshot.PayAmount != "" {
		currency := snapshot.Currency
		if currency == "" {
			currency = PaymentOrderCurrency(order)
		}
		if err := paymentAmountsMatch(snapshot.PayAmount, 0, formatPaymentAmountExact(order.PayAmount, currency), order.PayAmount, currency); err != nil {
			return fmt.Errorf("pay amount snapshot mismatch: %w", err)
		}
	}
	return nil
}

func psSnapshotIntValue(value any) int {
	switch typed := value.(type) {
	case int:
		return typed
	case int32:
		return int(typed)
	case int64:
		return int(typed)
	case float32:
		return int(typed)
	case float64:
		return int(typed)
	case string:
		n, err := strconv.Atoi(strings.TrimSpace(typed))
		if err == nil {
			return n
		}
	}
	return 0
}

func (s *PaymentService) resolveSnapshotOrderProviderInstance(ctx context.Context, order *dbent.PaymentOrder, snapshot *paymentOrderProviderSnapshot) (*dbent.PaymentProviderInstance, error) {
	if s == nil || s.entClient == nil || order == nil || snapshot == nil {
		return nil, nil
	}

	snapshotInstanceID := strings.TrimSpace(snapshot.ProviderInstanceID)
	columnInstanceID := strings.TrimSpace(psStringValue(order.ProviderInstanceID))
	if snapshotInstanceID == "" {
		snapshotInstanceID = columnInstanceID
	}
	if snapshotInstanceID == "" {
		return nil, fmt.Errorf("order %d provider snapshot is missing provider_instance_id", order.ID)
	}
	if columnInstanceID != "" && snapshot.ProviderInstanceID != "" && !strings.EqualFold(columnInstanceID, snapshot.ProviderInstanceID) {
		return nil, fmt.Errorf("order %d provider snapshot instance mismatch: snapshot=%s order=%s", order.ID, snapshot.ProviderInstanceID, columnInstanceID)
	}

	instID, err := strconv.ParseInt(snapshotInstanceID, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("order %d provider snapshot instance id is invalid: %s", order.ID, snapshotInstanceID)
	}

	inst, err := s.entClient.PaymentProviderInstance.Get(ctx, instID)
	if err != nil {
		if dbent.IsNotFound(err) {
			return nil, fmt.Errorf("order %d provider snapshot instance %s is missing", order.ID, snapshotInstanceID)
		}
		return nil, err
	}

	if snapshot.ProviderKey != "" && !strings.EqualFold(strings.TrimSpace(inst.ProviderKey), snapshot.ProviderKey) {
		return nil, fmt.Errorf("order %d provider snapshot key mismatch: snapshot=%s instance=%s", order.ID, snapshot.ProviderKey, inst.ProviderKey)
	}

	return inst, nil
}

func expectedNotificationProviderKeyForOrder(registry *payment.Registry, order *dbent.PaymentOrder, instanceProviderKey string) string {
	if order == nil {
		return strings.TrimSpace(instanceProviderKey)
	}

	orderProviderKey := psStringValue(order.ProviderKey)
	if snapshot := psOrderProviderSnapshot(order); snapshot != nil && snapshot.ProviderKey != "" {
		orderProviderKey = snapshot.ProviderKey
	}

	return expectedNotificationProviderKey(registry, order.PaymentType, orderProviderKey, instanceProviderKey)
}

func validateProviderSnapshotMetadata(order *dbent.PaymentOrder, providerKey string, metadata map[string]string) error {
	if order == nil || len(metadata) == 0 {
		return nil
	}

	snapshot := psOrderProviderSnapshot(order)
	if snapshot == nil {
		return nil
	}

	switch strings.TrimSpace(providerKey) {
	case payment.TypeWxpay:
		if expected := strings.TrimSpace(snapshot.MerchantAppID); expected != "" {
			actual := strings.TrimSpace(metadata["appid"])
			if actual == "" {
				return fmt.Errorf("wxpay notification missing appid")
			}
			if !strings.EqualFold(expected, actual) {
				return fmt.Errorf("wxpay appid mismatch: expected %s, got %s", expected, actual)
			}
		}
		if expected := strings.TrimSpace(snapshot.MerchantID); expected != "" {
			actual := strings.TrimSpace(metadata["mchid"])
			if actual == "" {
				return fmt.Errorf("wxpay notification missing mchid")
			}
			if !strings.EqualFold(expected, actual) {
				return fmt.Errorf("wxpay mchid mismatch: expected %s, got %s", expected, actual)
			}
		}
		if expected := strings.TrimSpace(snapshot.Currency); expected != "" {
			actual := strings.ToUpper(strings.TrimSpace(metadata["currency"]))
			if actual == "" {
				return fmt.Errorf("wxpay notification missing currency")
			}
			if !strings.EqualFold(expected, actual) {
				return fmt.Errorf("wxpay currency mismatch: expected %s, got %s", expected, actual)
			}
		}
		if actual := strings.TrimSpace(metadata["trade_state"]); actual != "" && !strings.EqualFold(actual, "SUCCESS") {
			return fmt.Errorf("wxpay trade_state mismatch: expected SUCCESS, got %s", actual)
		}
	case payment.TypeAlipay:
		if expected := strings.TrimSpace(snapshot.MerchantAppID); expected != "" {
			actual := strings.TrimSpace(metadata["app_id"])
			if actual == "" {
				return fmt.Errorf("alipay app_id missing")
			}
			if !strings.EqualFold(expected, actual) {
				return fmt.Errorf("alipay app_id mismatch: expected %s, got %s", expected, actual)
			}
		}
	case payment.TypeEasyPay:
		if expected := strings.TrimSpace(snapshot.MerchantID); expected != "" {
			actual := strings.TrimSpace(metadata["pid"])
			if actual == "" {
				return fmt.Errorf("easypay pid missing")
			}
			if !strings.EqualFold(expected, actual) {
				return fmt.Errorf("easypay pid mismatch: expected %s, got %s", expected, actual)
			}
		}
	case payment.TypeStripe:
		if expected := strings.TrimSpace(snapshot.Currency); expected != "" {
			actual := strings.ToUpper(strings.TrimSpace(metadata["currency"]))
			if actual == "" {
				return fmt.Errorf("stripe notification missing currency")
			}
			if !strings.EqualFold(expected, actual) {
				return fmt.Errorf("stripe currency mismatch: expected %s, got %s", expected, actual)
			}
		}
	case payment.TypeAirwallex:
		if expected := strings.TrimSpace(snapshot.MerchantID); expected != "" {
			actual := strings.TrimSpace(metadata["account_id"])
			if actual == "" {
				return fmt.Errorf("airwallex account_id missing")
			}
			if !strings.EqualFold(expected, actual) {
				return fmt.Errorf("airwallex account_id mismatch: expected %s, got %s", expected, actual)
			}
		}
		if expected := strings.TrimSpace(snapshot.Currency); expected != "" {
			actual := strings.ToUpper(strings.TrimSpace(metadata["currency"]))
			if actual == "" {
				return fmt.Errorf("airwallex notification missing currency")
			}
			if !strings.EqualFold(expected, actual) {
				return fmt.Errorf("airwallex currency mismatch: expected %s, got %s", expected, actual)
			}
		}
		if actual := strings.TrimSpace(metadata["status"]); actual != "" && !strings.EqualFold(actual, "SUCCEEDED") {
			return fmt.Errorf("airwallex status mismatch: expected SUCCEEDED, got %s", actual)
		}
	case payment.TypeInfini:
		if expected := strings.TrimSpace(snapshot.Currency); expected != "" {
			actual := strings.ToUpper(strings.TrimSpace(metadata["currency"]))
			if actual == "" {
				return fmt.Errorf("infini notification missing currency")
			}
			if !strings.EqualFold(expected, actual) {
				return fmt.Errorf("infini currency mismatch: expected %s, got %s", expected, actual)
			}
		}
		if expected := strings.TrimSpace(order.OutTradeNo); expected != "" {
			if actual := strings.TrimSpace(metadata["client_reference"]); actual != "" && !strings.EqualFold(expected, actual) {
				return fmt.Errorf("infini client_reference mismatch: expected %s, got %s", expected, actual)
			}
		}
		if expected := strings.TrimSpace(order.PaymentTradeNo); expected != "" {
			if actual := strings.TrimSpace(metadata["provider_order_id"]); actual != "" && !strings.EqualFold(expected, actual) {
				return fmt.Errorf("infini order_id mismatch: expected %s, got %s", expected, actual)
			}
		}
	}

	return nil
}

func providerMerchantIdentityMetadata(prov payment.Provider) map[string]string {
	if prov == nil {
		return nil
	}
	reporter, ok := prov.(payment.MerchantIdentityProvider)
	if !ok {
		return nil
	}
	return reporter.MerchantIdentityMetadata()
}
