package provider

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
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

func TestNewInfiniCoercesCNYConfigToUSD(t *testing.T) {
	t.Parallel()

	cfg := testInfiniConfig(infiniSandboxAPIBase)
	cfg["currency"] = "CNY"
	prov, err := NewInfini("1", cfg)
	require.NoError(t, err)
	require.Equal(t, "USD", prov.config["currency"])
	require.Equal(t, "USD", prov.currency())
	require.Equal(t, "USD", prov.MerchantIdentityMetadata()["currency"])
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
	require.Equal(t, "USD", captured.body.Currency)
	require.Equal(t, infiniDeterministicRequestID("order", "sub2_order_9", "7.50", "USD"), captured.body.RequestID)

	expected, err := infiniSignRequest(cfg["keyId"], cfg["secretKey"], http.MethodPost, "/v1/acquiring/order", captured.raw, func() time.Time { return fixed })
	require.NoError(t, err)
	require.Equal(t, expected["Date"], captured.date)
	require.Equal(t, expected["Authorization"], captured.auth)
	require.Equal(t, expected["Digest"], captured.digest)
}

func TestInfiniCreatePaymentSendsUSDWhenInstanceCurrencyIsCNY(t *testing.T) {
	t.Parallel()

	var captured infiniCreateOrderRequest
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.NoError(t, json.NewDecoder(r.Body).Decode(&captured))
		_ = json.NewEncoder(w).Encode(infiniCreateOrderResponse{
			OrderID:     "ord-cny",
			CheckoutURL: "https://checkout.infini.money/pay/cny",
		})
	}))
	defer server.Close()

	cfg := testInfiniConfig(infiniSandboxAPIBase)
	cfg["currency"] = "CNY"
	prov, err := NewInfini("1", cfg)
	require.NoError(t, err)
	prov.httpClient = server.Client()
	prov.httpClient.Transport = rewriteInfiniHost(server)

	resp, err := prov.CreatePayment(context.Background(), payment.CreatePaymentRequest{
		OrderID: "sub2_order_9",
		Amount:  "7.50",
		Subject: "Balance recharge",
	})
	require.NoError(t, err)
	require.Equal(t, "USD", resp.Currency)
	require.Equal(t, "7.50", captured.Amount)
	require.Equal(t, "USD", captured.Currency)
}

func TestInfiniCreatePaymentAcceptsWrappedAndCamelCaseResponses(t *testing.T) {
	t.Parallel()

	cases := [][]byte{
		[]byte(`{"code":0,"data":{"order_id":"ord-wrap","checkout_url":"https://checkout.infini.money/pay/wrap"}}`),
		[]byte(`{"code":"0","result":{"orderId":"ord-camel","checkoutUrl":"https://checkout.infini.money/pay/camel"}}`),
		[]byte(`{"success":true,"data":{"id":"ord-id","pay_url":"https://checkout.infini.money/pay/id"}}`),
		[]byte(`{"order":{"order_id":"ord-nested","checkout_url":"https://checkout.infini.money/pay/nested"}}`),
	}
	for _, body := range cases {
		parsed, err := parseInfiniCreateOrderResponse(body)
		require.NoError(t, err, string(body))
		require.NotEmpty(t, parsed.orderID(), string(body))
		require.NotEmpty(t, parsed.checkoutURL(), string(body))
	}
}

func TestInfiniCreatePaymentSurfacesBusinessErrorEnvelope(t *testing.T) {
	t.Parallel()

	_, err := parseInfiniCreateOrderResponse([]byte(`{"code":40006,"message":"amount must be greater than 0.1","detail":"crypto min 0.1"}`))
	require.ErrorContains(t, err, "40006")
	require.ErrorContains(t, err, "amount must be greater than 0.1")
}

func TestInfiniCreatePaymentReissuesCheckoutURLWhenOrderIDOnly(t *testing.T) {
	t.Parallel()

	var paths []string
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		switch r.URL.Path {
		case "/v1/acquiring/order":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"code": 0,
				"data": map[string]string{"order_id": "ord-reissue"},
			})
		case "/v1/acquiring/token/reissue":
			_ = json.NewEncoder(w).Encode(map[string]string{
				"order_id":     "ord-reissue",
				"checkout_url": "https://checkout.infini.money/pay/reissued",
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	prov, err := NewInfini("1", testInfiniConfig(infiniSandboxAPIBase))
	require.NoError(t, err)
	prov.httpClient = server.Client()
	prov.httpClient.Transport = rewriteInfiniHost(server)

	resp, err := prov.CreatePayment(context.Background(), payment.CreatePaymentRequest{
		OrderID: "sub2_order_reissue",
		Amount:  "7.50",
	})
	require.NoError(t, err)
	require.Equal(t, "ord-reissue", resp.TradeNo)
	require.Equal(t, "https://checkout.infini.money/pay/reissued", resp.PayURL)
	require.Equal(t, []string{"/v1/acquiring/order", "/v1/acquiring/token/reissue"}, paths)
}

func TestInfiniCreatePaymentIncludesBodyWhenCheckoutFieldsMissing(t *testing.T) {
	t.Parallel()

	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"code":0,"message":"success","data":{}}`))
	}))
	defer server.Close()

	prov, err := NewInfini("1", testInfiniConfig(infiniSandboxAPIBase))
	require.NoError(t, err)
	prov.httpClient = server.Client()
	prov.httpClient.Transport = rewriteInfiniHost(server)

	_, err = prov.CreatePayment(context.Background(), payment.CreatePaymentRequest{
		OrderID: "sub2_order_empty",
		Amount:  "7.50",
	})
	require.ErrorContains(t, err, "missing order_id or checkout_url")
	require.ErrorContains(t, err, `"message":"success"`)
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

// ---- Ed25519 (v2) authentication ----

const testInfiniV2KeyID = "9xQeWvG816bUx9EPjHmaT23yvVMuFbA1"

func testInfiniEd25519Key(t *testing.T) (ed25519.PublicKey, string) {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	der, err := x509.MarshalPKCS8PrivateKey(priv)
	require.NoError(t, err)
	return pub, string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der}))
}

func testInfiniV2Config(apiBase, privateKeyPEM string) map[string]string {
	return map[string]string{
		"keyId":         testInfiniV2KeyID,
		"privateKey":    privateKeyPEM,
		"webhookSecret": "whsec",
		"apiBase":       apiBase,
		"currency":      "USD",
	}
}

// infiniV2Capture records what the fake Infini server received and checks the
// Ed25519 signature the same way Infini does: over the request target and the
// raw body bytes it actually got.
type infiniV2Capture struct {
	method  string
	target  string
	headers http.Header
	raw     []byte
}

func (c infiniV2Capture) signingString() string {
	sum := sha256.Sum256(c.raw)
	return strings.Join([]string{
		"INFINI-OPENAPI-V2",
		c.headers.Get("X-API-Key-ID"),
		c.method,
		c.target,
		c.headers.Get("X-API-Timestamp"),
		c.headers.Get("X-API-Nonce"),
		"SHA-256=" + base64.StdEncoding.EncodeToString(sum[:]),
	}, "\n") + "\n"
}

func (c infiniV2Capture) requireValidSignature(t *testing.T, pub ed25519.PublicKey) {
	t.Helper()
	auth := c.headers.Get("Authorization")
	require.True(t, strings.HasPrefix(auth, `Ed25519 signature="`) && strings.HasSuffix(auth, `"`), "unexpected Authorization %q", auth)
	sig, err := base64.StdEncoding.DecodeString(strings.TrimSuffix(strings.TrimPrefix(auth, `Ed25519 signature="`), `"`))
	require.NoError(t, err)
	require.Len(t, sig, ed25519.SignatureSize)
	require.True(t, ed25519.Verify(pub, []byte(c.signingString()), sig), "signature does not verify over %q", c.signingString())

	sum := sha256.Sum256(c.raw)
	require.Equal(t, "SHA-256="+base64.StdEncoding.EncodeToString(sum[:]), c.headers.Get("Digest"))
	require.Equal(t, testInfiniV2KeyID, c.headers.Get("X-API-Key-ID"))
	nonce, err := base64.RawURLEncoding.DecodeString(c.headers.Get("X-API-Nonce"))
	require.NoError(t, err)
	require.Len(t, nonce, 16)
	// v2 does not use the legacy HMAC headers.
	require.Empty(t, c.headers.Get("Date"))
	require.NotContains(t, auth, "keyId=")
}

func newInfiniV2TestProvider(t *testing.T, handler func(capture infiniV2Capture, w http.ResponseWriter)) (*Infini, ed25519.PublicKey, *[]infiniV2Capture) {
	t.Helper()
	pub, privPEM := testInfiniEd25519Key(t)
	captures := &[]infiniV2Capture{}
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		capture := infiniV2Capture{method: r.Method, target: r.URL.RequestURI(), headers: r.Header.Clone(), raw: raw}
		*captures = append(*captures, capture)
		handler(capture, w)
	}))
	t.Cleanup(server.Close)

	prov, err := NewInfini("1", testInfiniV2Config(infiniSandboxAPIBase, privPEM))
	require.NoError(t, err)
	prov.httpClient = server.Client()
	prov.httpClient.Transport = rewriteInfiniHost(server)
	return prov, pub, captures
}

func TestNewInfiniRequiresPrivateKeyOrSecretKey(t *testing.T) {
	t.Parallel()

	cfg := testInfiniConfig(infiniSandboxAPIBase)
	delete(cfg, "secretKey")
	_, err := NewInfini("1", cfg)
	require.ErrorContains(t, err, "privateKey")
	require.ErrorContains(t, err, "secretKey")
}

func TestNewInfiniSelectsSchemeFromConfiguredCredential(t *testing.T) {
	t.Parallel()

	_, privPEM := testInfiniEd25519Key(t)

	legacy, err := NewInfini("1", testInfiniConfig(infiniSandboxAPIBase))
	require.NoError(t, err)
	require.False(t, legacy.usesEd25519())
	require.Equal(t, "/v1/acquiring/order", legacy.orderPath())
	require.Equal(t, "/v1/acquiring/token/reissue", legacy.reissuePath())

	v2, err := NewInfini("1", testInfiniV2Config(infiniSandboxAPIBase, privPEM))
	require.NoError(t, err)
	require.True(t, v2.usesEd25519())
	require.Equal(t, "/v2/acquiring/order", v2.orderPath())
	require.Equal(t, "/v2/acquiring/order/token/reissue", v2.reissuePath())

	// A leftover legacy secret must not keep an instance on v1 once the
	// Ed25519 key is configured.
	both := testInfiniV2Config(infiniSandboxAPIBase, privPEM)
	both["secretKey"] = "super-secret"
	mixed, err := NewInfini("1", both)
	require.NoError(t, err)
	require.True(t, mixed.usesEd25519())
}

func TestParseInfiniPrivateKeyAcceptsMangledPEM(t *testing.T) {
	t.Parallel()

	pub, privPEM := testInfiniEd25519Key(t)
	body := strings.TrimSpace(strings.NewReplacer("-----BEGIN PRIVATE KEY-----", "", "-----END PRIVATE KEY-----", "").Replace(privPEM))

	variants := map[string]string{
		"pem":             privPEM,
		"crlf":            strings.ReplaceAll(privPEM, "\n", "\r\n"),
		"literal newline": strings.ReplaceAll(strings.TrimSpace(privPEM), "\n", `\n`),
		"single line":     strings.ReplaceAll(strings.TrimSpace(privPEM), "\n", " "),
		"padded":          "\n\n  " + privPEM + "  \n",
		"bare base64":     body,
	}
	for name, input := range variants {
		key, err := parseInfiniPrivateKey(input)
		require.NoError(t, err, name)
		require.Equal(t, pub, key.Public(), name)
	}
}

func TestParseInfiniPrivateKeyRejectsWrongMaterialWithoutLeakingIt(t *testing.T) {
	t.Parallel()

	pub, privPEM := testInfiniEd25519Key(t)
	pubDER, err := x509.MarshalPKIXPublicKey(pub)
	require.NoError(t, err)
	publicPEM := string(pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: pubDER}))

	ecKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	ecDER, err := x509.MarshalPKCS8PrivateKey(ecKey)
	require.NoError(t, err)
	ecPEM := string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: ecDER}))

	cases := []struct {
		name, input, want string
	}{
		{"public key", publicPEM, "public key"},
		{"non-ed25519", ecPEM, "Ed25519"},
		{"encrypted", strings.ReplaceAll(privPEM, "PRIVATE KEY", "ENCRYPTED PRIVATE KEY"), "passphrase"},
		{"other block", strings.ReplaceAll(privPEM, "PRIVATE KEY", "OPENSSH PRIVATE KEY"), "PKCS#8"},
		{"mismatched armor", strings.Replace(privPEM, "END PRIVATE KEY", "END PUBLIC KEY", 1), "valid PEM"},
		{"truncated", privPEM[:len(privPEM)/2], "valid PEM"},
		{"legacy hmac secret", "super-secret", "PKCS#8"},
		{"random base64", base64.StdEncoding.EncodeToString([]byte("definitely not a key")), "PKCS#8"},
	}
	for _, tc := range cases {
		_, err := parseInfiniPrivateKey(tc.input)
		require.ErrorContains(t, err, tc.want, tc.name)
		for _, line := range strings.Fields(tc.input) {
			if len(line) >= 16 && !strings.HasPrefix(line, "-----") {
				require.NotContains(t, err.Error(), line, tc.name)
			}
		}
	}

	cfg := testInfiniV2Config(infiniSandboxAPIBase, publicPEM)
	_, err = NewInfini("1", cfg)
	require.ErrorContains(t, err, "public key")
}

func TestInfiniSignRequestV2BuildsDocumentedSigningString(t *testing.T) {
	t.Parallel()

	seed := bytes.Repeat([]byte{0x42}, ed25519.SeedSize)
	priv := ed25519.NewKeyFromSeed(seed)
	pub, ok := priv.Public().(ed25519.PublicKey)
	require.True(t, ok)
	nonceBytes := []byte{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 255}
	fixed := time.Unix(1795000000, 987654321)
	body := []byte(`{"order_id":"ord-1"}`)

	headers, err := infiniSignRequestV2(testInfiniV2KeyID, priv, "post", "/v2/acquiring/order/token/reissue", body, func() time.Time { return fixed }, bytes.NewReader(nonceBytes))
	require.NoError(t, err)
	require.Len(t, headers, 5)
	require.Equal(t, testInfiniV2KeyID, headers["X-API-Key-ID"])
	require.Equal(t, "1795000000", headers["X-API-Timestamp"])
	require.Equal(t, "AAECAwQFBgcICQoLDA0O_w", headers["X-API-Nonce"])
	sum := sha256.Sum256(body)
	require.Equal(t, "SHA-256="+base64.StdEncoding.EncodeToString(sum[:]), headers["Digest"])

	// Spelled out literally so a change to line order, separators or the
	// trailing newline cannot slip through.
	signingString := "INFINI-OPENAPI-V2\n" +
		testInfiniV2KeyID + "\n" +
		"POST\n" +
		"/v2/acquiring/order/token/reissue\n" +
		"1795000000\n" +
		"AAECAwQFBgcICQoLDA0O_w\n" +
		headers["Digest"] + "\n"
	sig, err := base64.StdEncoding.DecodeString(strings.TrimSuffix(strings.TrimPrefix(headers["Authorization"], `Ed25519 signature="`), `"`))
	require.NoError(t, err)
	require.True(t, ed25519.Verify(pub, []byte(signingString), sig))
	// Ed25519 is deterministic: same inputs, same signature.
	require.Equal(t, base64.StdEncoding.EncodeToString(ed25519.Sign(priv, []byte(signingString))), base64.StdEncoding.EncodeToString(sig))

	// An empty body still gets a digest: the hash of zero bytes.
	empty, err := infiniSignRequestV2(testInfiniV2KeyID, priv, http.MethodGet, "/v2/acquiring/order?order_id=x", nil, func() time.Time { return fixed }, bytes.NewReader(nonceBytes))
	require.NoError(t, err)
	require.Equal(t, "SHA-256=47DEQpj8HBSa+/TImW+5JCeuQeRkm5NMpJWZG3hSuFU=", empty["Digest"])

	_, err = infiniSignRequestV2("  ", priv, http.MethodGet, "/x", nil, nil, nil)
	require.ErrorContains(t, err, "keyId")
	_, err = infiniSignRequestV2(testInfiniV2KeyID, nil, http.MethodGet, "/x", nil, nil, nil)
	require.ErrorContains(t, err, "privateKey")
	_, err = infiniSignRequestV2(testInfiniV2KeyID, priv, http.MethodGet, "/x", nil, nil, bytes.NewReader([]byte{1, 2, 3}))
	require.ErrorContains(t, err, "nonce")
}

func TestInfiniV2CreatePaymentSignsExactBodyOnV2Path(t *testing.T) {
	t.Parallel()

	prov, pub, captures := newInfiniV2TestProvider(t, func(_ infiniV2Capture, w http.ResponseWriter) {
		// v2 documents a bare object, without the {code,data} envelope.
		_, _ = w.Write([]byte(`{"order_id":"ord-v2","request_id":"r","checkout_url":"https://checkout.infini.money/pay/v2","client_reference":"sub2_order_9","token":"tok"}`))
	})
	fixed := time.Unix(1795000123, 0)
	prov.now = func() time.Time { return fixed }

	resp, err := prov.CreatePayment(context.Background(), payment.CreatePaymentRequest{
		OrderID:   "sub2_order_9",
		Amount:    "7.50",
		Subject:   "Balance recharge",
		ReturnURL: "https://app.example/payment/result",
	})
	require.NoError(t, err)
	require.Equal(t, "ord-v2", resp.TradeNo)
	require.Equal(t, "https://checkout.infini.money/pay/v2", resp.PayURL)

	require.Len(t, *captures, 1)
	got := (*captures)[0]
	require.Equal(t, http.MethodPost, got.method)
	require.Equal(t, "/v2/acquiring/order", got.target)
	require.Equal(t, "1795000123", got.headers.Get("X-API-Timestamp"))
	require.Equal(t, "application/json", got.headers.Get("Content-Type"))
	got.requireValidSignature(t, pub)

	var body infiniCreateOrderRequest
	require.NoError(t, json.Unmarshal(got.raw, &body))
	require.Equal(t, "7.50", body.Amount)
	require.Equal(t, "USD", body.Currency)
	require.Equal(t, "sub2_order_9", body.ClientReference)
	// v2 rejects a request_id that is not a UUID.
	require.Regexp(t, `^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`, body.RequestID)
}

func TestInfiniV2QueryOrderSignsEscapedQueryAndEmptyBody(t *testing.T) {
	t.Parallel()

	prov, pub, captures := newInfiniV2TestProvider(t, func(_ infiniV2Capture, w http.ResponseWriter) {
		_, _ = w.Write([]byte(`{"order_id":"ord 1/2&x","status":"paid","amount":"7.5","currency":"USD","amount_confirmed":"7.5","client_reference":"sub2_order_9"}`))
	})

	resp, err := prov.QueryOrder(context.Background(), "ord 1/2&x")
	require.NoError(t, err)
	require.Equal(t, payment.ProviderStatusPaid, resp.Status)
	require.InDelta(t, 7.5, resp.Amount, 1e-9)

	require.Len(t, *captures, 1)
	got := (*captures)[0]
	require.Equal(t, http.MethodGet, got.method)
	// The signed target must be byte-for-byte what was sent, escapes included.
	require.Equal(t, "/v2/acquiring/order?order_id=ord+1%2F2%26x", got.target)
	require.Empty(t, got.raw)
	require.Equal(t, "SHA-256=47DEQpj8HBSa+/TImW+5JCeuQeRkm5NMpJWZG3hSuFU=", got.headers.Get("Digest"))
	got.requireValidSignature(t, pub)
}

func TestInfiniV2ReissuesCheckoutURLOnV2PathWithFreshNonce(t *testing.T) {
	t.Parallel()

	prov, pub, captures := newInfiniV2TestProvider(t, func(capture infiniV2Capture, w http.ResponseWriter) {
		if capture.target == "/v2/acquiring/order" {
			_, _ = w.Write([]byte(`{"order_id":"ord-v2"}`))
			return
		}
		_, _ = w.Write([]byte(`{"order_id":"ord-v2","checkout_url":"https://checkout.infini.money/pay/reissued","token":"tok"}`))
	})

	resp, err := prov.CreatePayment(context.Background(), payment.CreatePaymentRequest{OrderID: "sub2_order_10", Amount: "5.00"})
	require.NoError(t, err)
	require.Equal(t, "https://checkout.infini.money/pay/reissued", resp.PayURL)

	require.Len(t, *captures, 2)
	create, reissue := (*captures)[0], (*captures)[1]
	require.Equal(t, "/v2/acquiring/order/token/reissue", reissue.target)
	require.JSONEq(t, `{"order_id":"ord-v2"}`, string(reissue.raw))
	create.requireValidSignature(t, pub)
	reissue.requireValidSignature(t, pub)
	// Every request needs its own nonce; Infini rejects reuse.
	require.NotEqual(t, create.headers.Get("X-API-Nonce"), reissue.headers.Get("X-API-Nonce"))
}

func TestInfiniV2SurfacesAuthenticationFailure(t *testing.T) {
	t.Parallel()

	prov, _, _ := newInfiniV2TestProvider(t, func(_ infiniV2Capture, w http.ResponseWriter) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"code":401,"message":"Invalid Ed25519 authentication","detail":"signature mismatch"}`))
	})

	_, err := prov.QueryOrder(context.Background(), "ord-1")
	require.ErrorContains(t, err, "401")
	require.ErrorContains(t, err, "Invalid Ed25519 authentication")
}

func TestInfiniQueryOrderAcceptsReferenceStyleAmountFields(t *testing.T) {
	t.Parallel()

	cases := map[string]string{
		"string":        `{"order_id":"ord-1","status":"paid","order_currency":"USD","order_amount":"7.5"}`,
		"number":        `{"order_id":"ord-1","status":"paid","order_currency":"USD","order_amount":7.5}`,
		"object":        `{"order_id":"ord-1","status":"paid","order_currency":"USD","order_amount":{"amount":"7.5","currency":"USD"}}`,
		"object value":  `{"order_id":"ord-1","status":"paid","order_currency":"USD","order_amount":{"value":7.5}}`,
		"flat wins":     `{"order_id":"ord-1","status":"paid","amount":"7.5","currency":"USD","order_currency":"EUR","order_amount":"99"}`,
		"confirmed too": `{"order_id":"ord-1","status":"paid","order_currency":"USD","order_amount":"9","amount_confirmed":"7.5"}`,
	}
	for name, body := range cases {
		prov, _, _ := newInfiniV2TestProvider(t, func(_ infiniV2Capture, w http.ResponseWriter) {
			_, _ = w.Write([]byte(body))
		})
		resp, err := prov.QueryOrder(context.Background(), "ord-1")
		require.NoError(t, err, name)
		require.Equal(t, payment.ProviderStatusPaid, resp.Status, name)
		require.InDelta(t, 7.5, resp.Amount, 1e-9, name)
		require.Equal(t, "USD", resp.Metadata["currency"], name)
	}

	// An expired order is only ever settled by amount_confirmed; the listed
	// amount, in either shape, must not turn it into a paid order.
	prov, _, _ := newInfiniV2TestProvider(t, func(_ infiniV2Capture, w http.ResponseWriter) {
		_, _ = w.Write([]byte(`{"order_id":"ord-1","status":"expired","order_currency":"USD","order_amount":{"amount":"7.5"}}`))
	})
	resp, err := prov.QueryOrder(context.Background(), "ord-1")
	require.NoError(t, err)
	require.Equal(t, payment.ProviderStatusFailed, resp.Status)

	// A paid order whose amount cannot be read at all is an error, not a guess.
	prov, _, _ = newInfiniV2TestProvider(t, func(_ infiniV2Capture, w http.ResponseWriter) {
		_, _ = w.Write([]byte(`{"order_id":"ord-1","status":"paid","order_currency":"USD","order_amount":{"unknown":"7.5"}}`))
	})
	_, err = prov.QueryOrder(context.Background(), "ord-1")
	require.ErrorContains(t, err, "missing confirmed amount")
}
