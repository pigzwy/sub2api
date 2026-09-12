package provider

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/payment"
	"github.com/stretchr/testify/require"
)

func testInfiniConfig(apiBase string) map[string]string {
	return map[string]string{
		"keyId":         "merchant-001",
		"secretKey":     "super-secret",
		"webhookSecret": "whsec",
		"apiBase":       apiBase,
		"currency":      "USD",
	}
}

func TestNewInfiniNormalizesSandboxBaseAndCurrency(t *testing.T) {
	t.Parallel()

	prov, err := NewInfini("1", testInfiniConfig(infiniSandboxAPIBase+"/"))
	require.NoError(t, err)
	require.Equal(t, payment.TypeInfini, prov.ProviderKey())
	require.Equal(t, []payment.PaymentType{payment.TypeInfini}, prov.SupportedTypes())
	require.Equal(t, infiniSandboxAPIBase, prov.config["apiBase"])
	require.Equal(t, "USD", prov.config["currency"])
	require.Equal(t, "USD", prov.MerchantIdentityMetadata()["currency"])
}

func TestNewInfiniRejectsUnknownAPIHost(t *testing.T) {
	t.Parallel()

	_, err := NewInfini("1", testInfiniConfig("https://example.com"))
	require.ErrorContains(t, err, "openapi.infini.money")
}

func TestInfiniCreatePaymentSignsHostedCheckoutRequest(t *testing.T) {
	t.Parallel()

	var captured struct {
		method string
		path   string
		date   string
		auth   string
		digest string
		raw    []byte
		body   infiniCreateOrderRequest
	}
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		captured.method = r.Method
		captured.path = r.URL.RequestURI()
		captured.date = r.Header.Get("Date")
		captured.auth = r.Header.Get("Authorization")
		captured.digest = r.Header.Get("Digest")
		raw, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		captured.raw = raw
		require.NoError(t, json.Unmarshal(raw, &captured.body))
		_ = json.NewEncoder(w).Encode(infiniCreateOrderResponse{
			OrderID:     "ord-123",
			CheckoutURL: "https://checkout.infini.money/pay/xxxx",
		})
	}))
	defer server.Close()

	cfg := testInfiniConfig(infiniSandboxAPIBase)
	prov, err := NewInfini("1", cfg)
	require.NoError(t, err)
	prov.httpClient = server.Client()
	prov.httpClient.Transport = rewriteInfiniHost(server)
	fixed := time.Date(2026, 9, 12, 6, 0, 0, 0, time.UTC)
	prov.now = func() time.Time { return fixed }

	resp, err := prov.CreatePayment(context.Background(), payment.CreatePaymentRequest{
		OrderID:   "sub2_order_9",
		Amount:    "7.50",
		Subject:   "Balance recharge",
		ReturnURL: "https://app.example/payment/result",
	})
	require.NoError(t, err)
	require.Equal(t, "ord-123", resp.TradeNo)
	require.Equal(t, "https://checkout.infini.money/pay/xxxx", resp.PayURL)
	require.Equal(t, "USD", resp.Currency)
	require.Equal(t, "sandbox", resp.PaymentEnv)

	require.Equal(t, http.MethodPost, captured.method)
	require.Equal(t, "/v1/acquiring/order", captured.path)
	require.Equal(t, "7.50", captured.body.Amount)
	require.Equal(t, "sub2_order_9", captured.body.ClientReference)
	require.Equal(t, []int{1}, captured.body.PayMethods)
	require.Equal(t, infiniDeterministicRequestID("order", "sub2_order_9", "7.50", "USD"), captured.body.RequestID)

	expected, err := infiniSignRequest(cfg["keyId"], cfg["secretKey"], http.MethodPost, "/v1/acquiring/order", captured.raw, func() time.Time { return fixed })
	require.NoError(t, err)
	require.Equal(t, expected["Date"], captured.date)
	require.Equal(t, expected["Authorization"], captured.auth)
	require.Equal(t, expected["Digest"], captured.digest)
}

func TestInfiniVerifyNotificationCompletesPaidOrder(t *testing.T) {
	t.Parallel()

	prov, err := NewInfini("1", testInfiniConfig(infiniSandboxAPIBase))
	require.NoError(t, err)
	now := time.Unix(1700000000, 0)
	prov.now = func() time.Time { return now }

	body := `{"event":"order.completed","order_id":"ord-123","client_reference":"sub2_order_9","amount":"7.50","currency":"USD","status":"paid","amount_confirmed":"7.50"}`
	headers := infiniWebhookHeaders(t, body, "whsec", now)

	note, err := prov.VerifyNotification(context.Background(), body, headers)
	require.NoError(t, err)
	require.Equal(t, "ord-123", note.TradeNo)
	require.Equal(t, "sub2_order_9", note.OrderID)
	require.Equal(t, 7.5, note.Amount)
	require.Equal(t, payment.NotificationStatusSuccess, note.Status)
	require.Equal(t, "USD", note.Metadata["currency"])
}

func TestInfiniVerifyNotificationIgnoresProcessingAndRejectsBadSignature(t *testing.T) {
	t.Parallel()

	prov, err := NewInfini("1", testInfiniConfig(infiniSandboxAPIBase))
	require.NoError(t, err)
	now := time.Unix(1700000000, 0)
	prov.now = func() time.Time { return now }

	processing := `{"event":"order.processing","order_id":"ord-123","client_reference":"sub2_order_9","status":"processing"}`
	note, err := prov.VerifyNotification(context.Background(), processing, infiniWebhookHeaders(t, processing, "whsec", now))
	require.NoError(t, err)
	require.Nil(t, note)

	paid := `{"event":"order.completed","order_id":"ord-123","client_reference":"sub2_order_9","amount":"7.50","currency":"USD","status":"paid"}`
	_, err = prov.VerifyNotification(context.Background(), paid, infiniWebhookHeaders(t, paid, "other", now))
	require.ErrorContains(t, err, "invalid signature")

	expired := `{"event":"order.expired","order_id":"ord-123","client_reference":"sub2_order_9","status":"expired"}`
	note, err = prov.VerifyNotification(context.Background(), expired, infiniWebhookHeaders(t, expired, "whsec", now))
	require.NoError(t, err)
	require.Equal(t, payment.ProviderStatusFailed, note.Status)
	require.Equal(t, "sub2_order_9", note.OrderID)
}

func TestInfiniQueryOrderMapsPaidStatus(t *testing.T) {
	t.Parallel()

	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodGet, r.Method)
		require.Equal(t, "ord-123", r.URL.Query().Get("order_id"))
		_ = json.NewEncoder(w).Encode(infiniOrder{
			OrderID:         "ord-123",
			Status:          "paid",
			Amount:          "7.50",
			Currency:        "USD",
			AmountConfirmed: "7.50",
		})
	}))
	defer server.Close()

	prov, err := NewInfini("1", testInfiniConfig(infiniSandboxAPIBase))
	require.NoError(t, err)
	prov.httpClient = server.Client()
	prov.httpClient.Transport = rewriteInfiniHost(server)

	resp, err := prov.QueryOrder(context.Background(), "ord-123")
	require.NoError(t, err)
	require.Equal(t, payment.ProviderStatusPaid, resp.Status)
	require.Equal(t, 7.5, resp.Amount)
}

func TestInfiniVerifyNotificationLatePaymentExpiredWithConfirmedAmount(t *testing.T) {
	t.Parallel()

	prov, err := NewInfini("1", testInfiniConfig(infiniSandboxAPIBase))
	require.NoError(t, err)
	now := time.Unix(1700000000, 0)
	prov.now = func() time.Time { return now }

	body := `{"event":"order.late_payment","order_id":"ord-123","client_reference":"sub2_order_9","amount":"7.50","currency":"USD","status":"expired","amount_confirmed":"7.50"}`
	note, err := prov.VerifyNotification(context.Background(), body, infiniWebhookHeaders(t, body, "whsec", now))
	require.NoError(t, err)
	require.Equal(t, payment.NotificationStatusSuccess, note.Status)
	require.Equal(t, "7.5", note.AmountExact)
	require.Equal(t, 7.5, note.Amount)
	require.Equal(t, "evt-1", note.EventID)
}

func TestInfiniVerifyNotificationLatePaymentRejectsMissingConfirmedAmount(t *testing.T) {
	t.Parallel()

	prov, err := NewInfini("1", testInfiniConfig(infiniSandboxAPIBase))
	require.NoError(t, err)
	now := time.Unix(1700000000, 0)
	prov.now = func() time.Time { return now }

	body := `{"event":"order.late_payment","order_id":"ord-123","client_reference":"sub2_order_9","amount":"7.50","currency":"USD","status":"expired"}`
	_, err = prov.VerifyNotification(context.Background(), body, infiniWebhookHeaders(t, body, "whsec", now))
	require.ErrorContains(t, err, "amount_confirmed")
}

func TestInfiniVerifyNotificationExpiredWithoutConfirmedStaysFailed(t *testing.T) {
	t.Parallel()

	prov, err := NewInfini("1", testInfiniConfig(infiniSandboxAPIBase))
	require.NoError(t, err)
	now := time.Unix(1700000000, 0)
	prov.now = func() time.Time { return now }

	body := `{"event":"order.expired","order_id":"ord-123","client_reference":"sub2_order_9","amount":"7.50","currency":"USD","status":"expired"}`
	note, err := prov.VerifyNotification(context.Background(), body, infiniWebhookHeaders(t, body, "whsec", now))
	require.NoError(t, err)
	require.Equal(t, payment.ProviderStatusFailed, note.Status)
}

func TestInfiniVerifyNotificationExpiredWithConfirmedIsFulfillCandidate(t *testing.T) {
	t.Parallel()

	prov, err := NewInfini("1", testInfiniConfig(infiniSandboxAPIBase))
	require.NoError(t, err)
	now := time.Unix(1700000000, 0)
	prov.now = func() time.Time { return now }

	body := `{"event":"order.expired","order_id":"ord-123","client_reference":"sub2_order_9","amount":"7.50","currency":"USD","status":"expired","amount_confirmed":"7.50"}`
	note, err := prov.VerifyNotification(context.Background(), body, infiniWebhookHeaders(t, body, "whsec", now))
	require.NoError(t, err)
	require.Equal(t, payment.NotificationStatusSuccess, note.Status)
	require.Equal(t, 7.5, note.Amount)
}

func TestInfiniVerifyNotificationRejectsReplayAndTampering(t *testing.T) {
	t.Parallel()

	prov, err := NewInfini("1", testInfiniConfig(infiniSandboxAPIBase))
	require.NoError(t, err)
	now := time.Unix(1700000000, 0)
	prov.now = func() time.Time { return now }

	body := `{"event":"order.completed","order_id":"ord-123","client_reference":"sub2_order_9","amount":"7.50","currency":"USD","status":"paid","amount_confirmed":"7.50"}`
	headers := infiniWebhookHeaders(t, body, "whsec", now)

	tampered := strings.Replace(body, "7.50", "9.99", 1)
	_, err = prov.VerifyNotification(context.Background(), tampered, headers)
	require.ErrorContains(t, err, "invalid signature")

	stale := infiniWebhookHeaders(t, body, "whsec", now.Add(-10*time.Minute))
	_, err = prov.VerifyNotification(context.Background(), body, stale)
	require.ErrorContains(t, err, "timestamp")

	headers["x-webhook-event-id"] = "evt-other"
	_, err = prov.VerifyNotification(context.Background(), body, headers)
	require.ErrorContains(t, err, "invalid signature")
}

func TestInfiniQueryOrderExpiredUsesConfirmedAmountOnly(t *testing.T) {
	t.Parallel()

	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(infiniOrder{
			OrderID:         "ord-123",
			Status:          "expired",
			Amount:          "7.50",
			Currency:        "USD",
			AmountConfirmed: "7.50",
		})
	}))
	defer server.Close()

	prov, err := NewInfini("1", testInfiniConfig(infiniSandboxAPIBase))
	require.NoError(t, err)
	prov.httpClient = server.Client()
	prov.httpClient.Transport = rewriteInfiniHost(server)

	resp, err := prov.QueryOrder(context.Background(), "ord-123")
	require.NoError(t, err)
	require.Equal(t, payment.ProviderStatusPaid, resp.Status)
	require.Equal(t, 7.5, resp.Amount)
}

func TestInfiniQueryOrderExpiredWithoutConfirmedIsFailed(t *testing.T) {
	t.Parallel()

	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(infiniOrder{
			OrderID:  "ord-123",
			Status:   "expired",
			Amount:   "7.50",
			Currency: "USD",
		})
	}))
	defer server.Close()

	prov, err := NewInfini("1", testInfiniConfig(infiniSandboxAPIBase))
	require.NoError(t, err)
	prov.httpClient = server.Client()
	prov.httpClient.Transport = rewriteInfiniHost(server)

	resp, err := prov.QueryOrder(context.Background(), "ord-123")
	require.NoError(t, err)
	require.Equal(t, payment.ProviderStatusFailed, resp.Status)
}

func TestInfiniQueryOrderPartialPaidIsNotPaid(t *testing.T) {
	t.Parallel()

	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(infiniOrder{
			OrderID:         "ord-123",
			Status:          "partial_paid",
			Amount:          "7.50",
			Currency:        "USD",
			AmountConfirmed: "3.00",
		})
	}))
	defer server.Close()

	prov, err := NewInfini("1", testInfiniConfig(infiniSandboxAPIBase))
	require.NoError(t, err)
	prov.httpClient = server.Client()
	prov.httpClient.Transport = rewriteInfiniHost(server)

	resp, err := prov.QueryOrder(context.Background(), "ord-123")
	require.NoError(t, err)
	require.Equal(t, payment.ProviderStatusFailed, resp.Status)
}

func TestInfiniRefundIsUnsupported(t *testing.T) {
	t.Parallel()

	prov, err := NewInfini("1", testInfiniConfig(infiniSandboxAPIBase))
	require.NoError(t, err)
	_, err = prov.Refund(context.Background(), payment.RefundRequest{TradeNo: "ord-123", Amount: "7.50"})
	require.ErrorContains(t, err, "not supported")
}

func infiniWebhookHeaders(t *testing.T, body, secret string, now time.Time) map[string]string {
	t.Helper()
	ts := strconv.FormatInt(now.Unix(), 10)
	eventID := "evt-1"
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(ts + "." + eventID + "." + body))
	return map[string]string{
		"x-webhook-timestamp": ts,
		"x-webhook-event-id":  eventID,
		"x-webhook-signature": hex.EncodeToString(mac.Sum(nil)),
	}
}

func rewriteInfiniHost(server *httptest.Server) http.RoundTripper {
	base := server.Client().Transport
	if base == nil {
		base = http.DefaultTransport
	}
	return roundTripFunc(func(req *http.Request) (*http.Response, error) {
		cloned := req.Clone(req.Context())
		u, _ := url.Parse(server.URL)
		cloned.URL.Scheme = u.Scheme
		cloned.URL.Host = u.Host
		cloned.Host = u.Host
		return base.RoundTrip(cloned)
	})
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}
