//go:build unit

package service

import (
	"context"
	"strconv"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/payment"
	"github.com/stretchr/testify/require"
)

// offlineEasyPayLoadBalancer returns a decrypted EasyPay popup instance.
// popup mode only builds a hosted URL and does not call a live gateway.
type offlineEasyPayLoadBalancer struct{}

func (offlineEasyPayLoadBalancer) GetInstanceConfig(context.Context, int64) (map[string]string, error) {
	return map[string]string{}, nil
}

func (offlineEasyPayLoadBalancer) SelectInstance(
	context.Context,
	string,
	payment.PaymentType,
	payment.Strategy,
	float64,
) (*payment.InstanceSelection, error) {
	return &payment.InstanceSelection{
		InstanceID:     "offline-easypay",
		ProviderKey:    payment.TypeEasyPay,
		SupportedTypes: payment.TypeAlipay,
		PaymentMode:    "popup",
		Config: map[string]string{
			"pid":         "1000",
			"pkey":        "offline-test-key",
			"apiBase":     "https://easypay.test",
			"notifyUrl":   "https://app.example.com/api/v1/payment/easypay/notify",
			"returnUrl":   "https://app.example.com/payment/result",
			"paymentMode": "popup",
		},
	}, nil
}

func TestCreateOrderSnapshotsBonusCreditBeforeFulfillment(t *testing.T) {
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)
	ensurePaymentAuditOrderActionUniqueIndex(t, ctx, client)

	user, err := client.User.Create().
		SetEmail("create-order-credit-" + strconv.FormatInt(time.Now().UnixNano(), 10) + "@example.com").
		SetPasswordHash("hash").
		SetUsername("create-order-credit-user").
		Save(ctx)
	require.NoError(t, err)

	enabled := true
	multiplier := 0.14
	feeRate := 0.0
	packages := []RechargePackage{{
		ID:     "cny200",
		Amount: 200,
		Bonus:  5,
		Name:   "加赠",
	}}
	settings := &paymentConfigSettingRepoStub{values: map[string]string{
		SettingMinRechargeAmount: "1",
	}}
	configService := &PaymentConfigService{entClient: client, settingRepo: settings}
	require.NoError(t, configService.UpdatePaymentConfig(ctx, UpdatePaymentConfigRequest{
		Enabled:                   &enabled,
		BalanceRechargeMultiplier: &multiplier,
		BalanceRechargePackages:   &packages,
		RechargeFeeRate:           &feeRate,
	}))

	stored, err := configService.GetPaymentConfig(ctx)
	require.NoError(t, err)
	require.True(t, stored.Enabled)
	require.InDelta(t, 0.14, stored.BalanceRechargeMultiplier, 1e-9)
	require.InDelta(t, 0, stored.RechargeFeeRate, 1e-9)
	require.Len(t, stored.BalanceRechargePackages, 1)
	require.InDelta(t, 200, stored.BalanceRechargePackages[0].Amount, 1e-9)
	require.InDelta(t, 5, stored.BalanceRechargePackages[0].Bonus, 1e-9)

	balance := 0.0
	userRepo := &mockUserRepo{getByIDUser: &User{
		ID:       user.ID,
		Email:    user.Email,
		Username: user.Username,
		Status:   payment.EntityStatusActive,
	}}
	userRepo.updateBalanceFn = func(_ context.Context, id int64, amount float64) error {
		require.Equal(t, user.ID, id)
		balance += amount
		return nil
	}
	redeemRepo := &paymentFulfillmentRedeemRepo{}
	cache := &paymentFulfillmentRedeemCacheStub{}
	svc := &PaymentService{
		entClient:     client,
		configService: configService,
		userRepo:      userRepo,
		loadBalancer:  offlineEasyPayLoadBalancer{},
		redeemService: NewRedeemService(redeemRepo, userRepo, nil, cache, nil, client, nil, nil),
	}

	resp, err := svc.CreateOrder(ctx, CreateOrderRequest{
		UserID:      user.ID,
		Amount:      200,
		PaymentType: payment.TypeAlipay,
		OrderType:   payment.OrderTypeBalance,
		ClientIP:    "127.0.0.1",
		SrcHost:     "app.example.com",
	})
	require.NoError(t, err)
	require.NotNil(t, resp)

	order, err := client.PaymentOrder.Get(ctx, resp.OrderID)
	require.NoError(t, err)
	require.InDelta(t, 33.0, order.Amount, 1e-8)
	require.InDelta(t, 200.0, order.PayAmount, 1e-8)
	require.Equal(t, payment.OrderTypeBalance, order.OrderType)
	require.Equal(t, OrderStatusPending, order.Status)

	changedMultiplier := 1.0
	changedPackages := []RechargePackage{{
		ID:     "cny200",
		Amount: 200,
		Bonus:  80,
		Name:   "加赠",
	}}
	require.NoError(t, configService.UpdatePaymentConfig(ctx, UpdatePaymentConfigRequest{
		BalanceRechargeMultiplier: &changedMultiplier,
		BalanceRechargePackages:   &changedPackages,
	}))
	live, err := configService.GetPaymentConfig(ctx)
	require.NoError(t, err)
	require.NotEqual(t, 33.0, calculateCreditedBalance(200, live.BalanceRechargeMultiplier, live.BalanceRechargePackages))

	mismatch := &payment.PaymentNotification{
		TradeNo: "easypay-bonus-mismatch",
		OrderID: order.OutTradeNo,
		Amount:  199,
		Status:  payment.NotificationStatusSuccess,
	}
	require.ErrorContains(t, svc.HandlePaymentNotification(ctx, mismatch, payment.TypeEasyPay), "amount mismatch")
	require.InDelta(t, 0, balance, 1e-8)
	pending, err := client.PaymentOrder.Get(ctx, order.ID)
	require.NoError(t, err)
	require.Equal(t, OrderStatusPending, pending.Status)
	require.InDelta(t, 33.0, pending.Amount, 1e-8)

	success := &payment.PaymentNotification{
		TradeNo: "easypay-bonus-credit",
		OrderID: order.OutTradeNo,
		Amount:  200,
		Status:  payment.NotificationStatusSuccess,
	}
	require.NoError(t, svc.HandlePaymentNotification(ctx, success, payment.TypeEasyPay))
	require.InDelta(t, 33.0, balance, 1e-8)

	require.NoError(t, svc.HandlePaymentNotification(ctx, success, payment.TypeEasyPay))
	require.InDelta(t, 33.0, balance, 1e-8)

	completed, err := client.PaymentOrder.Get(ctx, order.ID)
	require.NoError(t, err)
	require.Equal(t, OrderStatusCompleted, completed.Status)
	require.InDelta(t, 33.0, completed.Amount, 1e-8)
	require.InDelta(t, 200.0, completed.PayAmount, 1e-8)
}
