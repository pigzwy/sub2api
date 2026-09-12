package service

import (
	"context"
	"fmt"

	"github.com/Wei-Shaw/sub2api/internal/payment"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/shopspring/decimal"
)

type QuoteOrderResponse struct {
	OrderType      string  `json:"order_type"`
	PaymentType    string  `json:"payment_type"`
	PackageAmount  string  `json:"package_amount"`
	CreditAmount   string  `json:"credit_amount"`
	PayAmount      string  `json:"pay_amount"`
	PayAmountValue float64 `json:"pay_amount_value"`
	FeeAmount      string  `json:"fee_amount"`
	FeeRate        float64 `json:"fee_rate"`
	FxRate         float64 `json:"fx_rate"`
	FxConverted    bool    `json:"fx_converted"`
	Currency       string  `json:"currency"`
}

func (s *PaymentService) QuoteOrder(ctx context.Context, req CreateOrderRequest) (*QuoteOrderResponse, error) {
	if req.OrderType == "" {
		req.OrderType = payment.OrderTypeBalance
	}
	if normalized := NormalizeVisibleMethod(req.PaymentType); normalized != "" {
		req.PaymentType = normalized
	}
	cfg, err := s.configService.GetPaymentConfig(ctx)
	if err != nil {
		return nil, fmt.Errorf("get payment config: %w", err)
	}
	if !cfg.Enabled {
		return nil, infraerrors.Forbidden("PAYMENT_DISABLED", "payment system is disabled")
	}
	if err := validateSubscriptionUSDToCNYRate(cfg.SubscriptionUSDToCNYRate); err != nil {
		return nil, err
	}
	if err := validateSubscriptionUSDToCNYRate(cfg.USDTUSDToCNYRate); err != nil {
		return nil, err
	}
	plan, err := s.validateOrderInput(ctx, req, cfg)
	if err != nil {
		return nil, err
	}

	orderAmount := req.Amount
	limitAmount := req.Amount
	if plan != nil {
		orderAmount = plan.Price
		limitAmount = plan.Price
	} else if req.OrderType == payment.OrderTypeBalance {
		orderAmount = calculateCreditedBalance(req.Amount, cfg.BalanceRechargeMultiplier, cfg.BalanceRechargePackages)
	}

	methodCurrency := payment.DefaultPaymentCurrency
	if s.configService != nil {
		methodCurrency, err = s.configService.ValidateMethodCurrencyConsistency(ctx, req.PaymentType)
		if err != nil {
			return nil, err
		}
	}

	fxRate := resolvePayFXRate(cfg, req.OrderType, req.PaymentType, methodCurrency)
	payAmountStr, payAmount, err := calculateCreateOrderPayAmountForOrderType(
		limitAmount, cfg.RechargeFeeRate, methodCurrency, req.OrderType, req.PaymentType, fxRate,
	)
	if err != nil {
		return nil, err
	}

	baseDec, _ := gatewayBaseAmountDecimal(limitAmount, fxRate, methodCurrency, req.OrderType, req.PaymentType)
	digits := int32(payment.CurrencyMaxFractionDigits(methodCurrency))
	payDec, err := decimal.NewFromString(payAmountStr)
	if err != nil {
		return nil, fmt.Errorf("quote pay amount: %w", err)
	}
	feeDec := payDec.Sub(baseDec.Round(digits))
	if feeDec.IsNegative() {
		feeDec = decimal.Zero
	}
	feeAmount := feeDec.StringFixed(digits)
	fxConverted := quoteUsesFX(req.OrderType, req.PaymentType, methodCurrency, fxRate)

	return &QuoteOrderResponse{
		OrderType:      req.OrderType,
		PaymentType:    req.PaymentType,
		PackageAmount:  decimal.NewFromFloat(limitAmount).StringFixed(2),
		CreditAmount:   decimal.NewFromFloat(orderAmount).StringFixed(2),
		PayAmount:      payAmountStr,
		PayAmountValue: payAmount,
		FeeAmount:      feeAmount,
		FeeRate:        cfg.RechargeFeeRate,
		FxRate:         fxRate,
		FxConverted:    fxConverted,
		Currency:       methodCurrency,
	}, nil
}

func quoteUsesFX(orderType, paymentType, currency string, rate float64) bool {
	if rate <= 0 {
		return false
	}
	switch orderType {
	case payment.OrderTypeSubscription:
		return currency == payment.DefaultPaymentCurrency
	case payment.OrderTypeBalance:
		return shouldConvertBalancePayAmountToUSD(paymentType, currency)
	default:
		return false
	}
}
