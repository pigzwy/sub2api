package provider

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/payment"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

const (
	infiniProdAPIBase       = "https://openapi.infini.money"
	infiniSandboxAPIBase    = "https://openapi-sandbox.infini.money"
	infiniHTTPTimeout       = 15 * time.Second
	infiniMaxResponseSize   = 1 << 20
	infiniMaxErrorSummary   = 512
	infiniWebhookTolerance  = 5 * time.Minute
	infiniDefaultPayMethod  = 1 // crypto / on-chain stablecoin
	infiniEventOrderCreated = "order.created"
	infiniEventOrderCreate  = "order.create"
	infiniEventProcessing   = "order.processing"
	infiniEventCompleted    = "order.completed"
	infiniEventExpired      = "order.expired"
	infiniEventLatePayment  = "order.late_payment"
	infiniStatusPaid        = "paid"
	infiniStatusExpired     = "expired"
	infiniStatusPartialPaid = "partial_paid"
	infiniStatusPending     = "pending"
	infiniStatusProcessing  = "processing"
)

// Infini implements hosted-checkout USDT/stablecoin payments.
type Infini struct {
	instanceID string
	config     map[string]string
	httpClient *http.Client
	now        func() time.Time
}

func NewInfini(instanceID string, config map[string]string) (*Infini, error) {
	for _, k := range []string{"keyId", "secretKey", "webhookSecret", "apiBase"} {
		if strings.TrimSpace(config[k]) == "" {
			return nil, fmt.Errorf("infini config missing required key: %s", k)
		}
	}
	cfg := cloneStringMap(config)
	apiBase, err := normalizeInfiniAPIBase(cfg["apiBase"])
	if err != nil {
		return nil, err
	}
	cfg["apiBase"] = apiBase
	if strings.TrimSpace(cfg["currency"]) == "" {
		cfg["currency"] = "USD"
	}
	currency, err := payment.NormalizePaymentCurrency(cfg["currency"])
	if err != nil {
		return nil, fmt.Errorf("infini config currency: %w", err)
	}
	cfg["currency"] = currency
	if _, err := parseInfiniPayMethods(cfg["payMethods"]); err != nil {
		return nil, err
	}
	return &Infini{
		instanceID: instanceID,
		config:     cfg,
		httpClient: &http.Client{Timeout: infiniHTTPTimeout},
		now:        time.Now,
	}, nil
}

func normalizeInfiniAPIBase(raw string) (string, error) {
	base := strings.TrimSpace(raw)
	if base == "" {
		return "", fmt.Errorf("infini apiBase is required")
	}
	parsed, err := url.Parse(base)
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" {
		return "", fmt.Errorf("infini apiBase must be an HTTPS URL")
	}
	host := strings.ToLower(parsed.Host)
	if host != "openapi.infini.money" && host != "openapi-sandbox.infini.money" {
		return "", fmt.Errorf("infini apiBase host must be openapi.infini.money or openapi-sandbox.infini.money")
	}
	parsed.RawQuery = ""
	parsed.Fragment = ""
	parsed.RawPath = ""
	parsed.Path = strings.TrimRight(parsed.Path, "/")
	if parsed.Path != "" {
		return "", fmt.Errorf("infini apiBase must not include a path")
	}
	return parsed.Scheme + "://" + parsed.Host, nil
}

func parseInfiniPayMethods(raw string) ([]int, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return []int{infiniDefaultPayMethod}, nil
	}
	parts := strings.Split(raw, ",")
	methods := make([]int, 0, len(parts))
	seen := map[int]struct{}{}
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		n, err := strconv.Atoi(part)
		if err != nil || n <= 0 {
			return nil, fmt.Errorf("infini payMethods must be a comma-separated list of positive integers")
		}
		if _, ok := seen[n]; ok {
			continue
		}
		seen[n] = struct{}{}
		methods = append(methods, n)
	}
	if len(methods) == 0 {
		return []int{infiniDefaultPayMethod}, nil
	}
	return methods, nil
}

func (i *Infini) Name() string        { return "INFINI Stablecoin Payment" }
func (i *Infini) ProviderKey() string { return payment.TypeInfini }
func (i *Infini) SupportedTypes() []payment.PaymentType {
	return []payment.PaymentType{payment.TypeInfini}
}

func (i *Infini) MerchantIdentityMetadata() map[string]string {
	if i == nil {
		return nil
	}
	return map[string]string{"currency": i.currency()}
}

func (i *Infini) currency() string {
	if i == nil {
		return "USD"
	}
	currency, err := payment.NormalizePaymentCurrency(i.config["currency"])
	if err != nil {
		return "USD"
	}
	return currency
}

func (i *Infini) CreatePayment(ctx context.Context, req payment.CreatePaymentRequest) (*payment.CreatePaymentResponse, error) {
	amount, err := decimal.NewFromString(req.Amount)
	if err != nil || amount.LessThanOrEqual(decimal.Zero) {
		return nil, fmt.Errorf("infini create payment: invalid amount %s", req.Amount)
	}
	payMethods, err := parseInfiniPayMethods(i.config["payMethods"])
	if err != nil {
		return nil, err
	}
	currency := i.currency()
	payload := infiniCreateOrderRequest{
		Amount:          payment.FormatAmountForCurrency(amount.InexactFloat64(), currency),
		RequestID:       infiniDeterministicRequestID("order", req.OrderID, req.Amount, currency),
		ClientReference: req.OrderID,
		OrderDesc:       strings.TrimSpace(req.Subject),
		SuccessURL:      strings.TrimSpace(req.ReturnURL),
		FailureURL:      strings.TrimSpace(req.ReturnURL),
		PayMethods:      payMethods,
		Currency:        currency,
	}

	var resp infiniCreateOrderResponse
	if err := i.doJSON(ctx, http.MethodPost, "/v1/acquiring/order", payload, &resp); err != nil {
		return nil, fmt.Errorf("infini create payment: %w", err)
	}
	if strings.TrimSpace(resp.OrderID) == "" || strings.TrimSpace(resp.CheckoutURL) == "" {
		return nil, fmt.Errorf("infini create payment: missing order_id or checkout_url")
	}
	return &payment.CreatePaymentResponse{
		TradeNo:    resp.OrderID,
		PayURL:     resp.CheckoutURL,
		Currency:   currency,
		PaymentEnv: i.checkoutEnv(),
		ResultType: payment.CreatePaymentResultOrderCreated,
	}, nil
}

func (i *Infini) QueryOrder(ctx context.Context, tradeNo string) (*payment.QueryOrderResponse, error) {
	orderID := strings.TrimSpace(tradeNo)
	if orderID == "" {
		return nil, fmt.Errorf("infini query order: missing order id")
	}
	path := "/v1/acquiring/order?order_id=" + url.QueryEscape(orderID)
	var resp infiniOrder
	if err := i.doJSON(ctx, http.MethodGet, path, nil, &resp); err != nil {
		return nil, fmt.Errorf("infini query order: %w", err)
	}
	amount, _ := parseInfiniAmount(resp.AmountConfirmed, resp.Amount)
	return &payment.QueryOrderResponse{
		TradeNo:  firstNonEmpty(resp.OrderID, orderID),
		Status:   infiniProviderStatus(resp.Status),
		Amount:   amount,
		Metadata: i.orderMetadata(resp),
	}, nil
}

func (i *Infini) VerifyNotification(_ context.Context, rawBody string, headers map[string]string) (*payment.PaymentNotification, error) {
	now := time.Now()
	if i != nil && i.now != nil {
		now = i.now()
	}
	if err := verifyInfiniWebhookSignature(rawBody, headers, i.config["webhookSecret"], now); err != nil {
		return nil, err
	}

	var event infiniWebhookEvent
	if err := json.Unmarshal([]byte(rawBody), &event); err != nil {
		return nil, fmt.Errorf("infini parse webhook: %w", err)
	}
	switch strings.ToLower(strings.TrimSpace(event.Event)) {
	case infiniEventCompleted, infiniEventLatePayment, infiniEventExpired:
	case infiniEventOrderCreated, infiniEventOrderCreate, infiniEventProcessing:
		return nil, nil
	default:
		return nil, nil
	}

	orderID := strings.TrimSpace(event.ClientReference)
	if orderID == "" {
		return nil, fmt.Errorf("infini webhook missing client_reference")
	}
	tradeNo := strings.TrimSpace(event.OrderID)
	if tradeNo == "" {
		return nil, fmt.Errorf("infini webhook missing order_id")
	}

	var status string
	switch {
	case infiniWebhookPaid(event):
		status = payment.NotificationStatusSuccess
	case strings.EqualFold(event.Event, infiniEventExpired) || strings.EqualFold(event.Status, infiniStatusExpired):
		status = payment.ProviderStatusFailed
	default:
		return nil, nil
	}

	amount, err := parseInfiniAmount(event.AmountConfirmed, event.Amount)
	if err != nil && status == payment.NotificationStatusSuccess {
		return nil, fmt.Errorf("infini webhook invalid amount: %w", err)
	}

	return &payment.PaymentNotification{
		TradeNo:  tradeNo,
		OrderID:  orderID,
		Amount:   amount,
		Status:   status,
		RawData:  rawBody,
		Metadata: i.eventMetadata(event),
	}, nil
}

func (i *Infini) Refund(_ context.Context, _ payment.RefundRequest) (*payment.RefundResponse, error) {
	return nil, fmt.Errorf("infini refund is not supported")
}

func (i *Infini) checkoutEnv() string {
	if strings.EqualFold(i.config["apiBase"], infiniProdAPIBase) {
		return "prod"
	}
	return "sandbox"
}

func (i *Infini) orderMetadata(order infiniOrder) map[string]string {
	return map[string]string{
		"currency": strings.ToUpper(strings.TrimSpace(order.Currency)),
		"status":   strings.ToLower(strings.TrimSpace(order.Status)),
	}
}

func (i *Infini) eventMetadata(event infiniWebhookEvent) map[string]string {
	return map[string]string{
		"currency": strings.ToUpper(strings.TrimSpace(event.Currency)),
		"status":   strings.ToLower(strings.TrimSpace(event.Status)),
		"event":    strings.ToLower(strings.TrimSpace(event.Event)),
	}
}

func (i *Infini) doJSON(ctx context.Context, method, path string, payload any, out any) error {
	var bodyBytes []byte
	if payload != nil {
		b, err := json.Marshal(payload)
		if err != nil {
			return err
		}
		bodyBytes = b
	}
	headers, err := infiniSignRequest(i.config["keyId"], i.config["secretKey"], method, path, bodyBytes, i.now)
	if err != nil {
		return err
	}

	var body io.Reader
	if len(bodyBytes) > 0 {
		body = bytes.NewReader(bodyBytes)
		headers["Content-Type"] = "application/json"
	}
	req, err := http.NewRequestWithContext(ctx, method, i.config["apiBase"]+path, body)
	if err != nil {
		return err
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	client := i.httpClient
	if client == nil {
		client = &http.Client{Timeout: infiniHTTPTimeout}
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	respBody, err := io.ReadAll(io.LimitReader(resp.Body, infiniMaxResponseSize))
	if err != nil {
		return err
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, summarizeInfiniResponse(respBody))
	}
	if out == nil || len(bytes.TrimSpace(respBody)) == 0 {
		return nil
	}
	if err := json.Unmarshal(respBody, out); err != nil {
		return fmt.Errorf("parse response: %w", err)
	}
	return nil
}

func infiniSignRequest(keyID, secretKey, method, path string, body []byte, now func() time.Time) (map[string]string, error) {
	if now == nil {
		now = time.Now
	}
	gmtTime := now().UTC().Format("Mon, 02 Jan 2006 15:04:05 GMT")
	signingString := fmt.Sprintf("%s\n%s %s\ndate: %s\n", keyID, strings.ToUpper(method), path, gmtTime)
	mac := hmac.New(sha256.New, []byte(secretKey))
	_, _ = mac.Write([]byte(signingString))
	signature := base64.StdEncoding.EncodeToString(mac.Sum(nil))
	headers := map[string]string{
		"Date": gmtTime,
		"Authorization": fmt.Sprintf(
			`Signature keyId="%s",algorithm="hmac-sha256",headers="@request-target date",signature="%s"`,
			keyID, signature,
		),
	}
	if len(body) > 0 {
		sum := sha256.Sum256(body)
		headers["Digest"] = "SHA-256=" + base64.StdEncoding.EncodeToString(sum[:])
	}
	return headers, nil
}

func verifyInfiniWebhookSignature(rawBody string, headers map[string]string, secret string, now time.Time) error {
	secret = strings.TrimSpace(secret)
	if secret == "" {
		return fmt.Errorf("infini webhookSecret not configured")
	}
	timestamp := strings.TrimSpace(headerCI(headers, "x-webhook-timestamp"))
	eventID := strings.TrimSpace(headerCI(headers, "x-webhook-event-id"))
	signature := strings.TrimSpace(headerCI(headers, "x-webhook-signature"))
	if timestamp == "" || eventID == "" || signature == "" {
		return fmt.Errorf("infini notification missing webhook signature headers")
	}

	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(timestamp + "." + eventID + "." + rawBody))
	expected := hex.EncodeToString(mac.Sum(nil))
	if !hmac.Equal([]byte(strings.ToLower(expected)), []byte(strings.ToLower(signature))) {
		return fmt.Errorf("infini invalid signature")
	}

	ts, err := strconv.ParseInt(timestamp, 10, 64)
	if err != nil || ts <= 0 {
		return fmt.Errorf("infini invalid webhook timestamp")
	}
	if now.IsZero() {
		now = time.Now()
	}
	if diff := now.Sub(time.Unix(ts, 0)).Abs(); diff > infiniWebhookTolerance {
		return fmt.Errorf("infini webhook timestamp outside tolerance")
	}
	return nil
}

func headerCI(headers map[string]string, key string) string {
	if headers == nil {
		return ""
	}
	if v, ok := headers[key]; ok {
		return v
	}
	return headers[strings.ToLower(key)]
}

func infiniWebhookPaid(event infiniWebhookEvent) bool {
	eventName := strings.ToLower(strings.TrimSpace(event.Event))
	status := strings.ToLower(strings.TrimSpace(event.Status))
	if eventName != infiniEventCompleted && eventName != infiniEventLatePayment {
		return false
	}
	return status == infiniStatusPaid || status == ""
}

func infiniProviderStatus(status string) string {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case infiniStatusPaid:
		return payment.ProviderStatusPaid
	case infiniStatusExpired, infiniStatusPartialPaid:
		return payment.ProviderStatusFailed
	default:
		return payment.ProviderStatusPending
	}
}

func parseInfiniAmount(values ...string) (float64, error) {
	for _, raw := range values {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}
		amount, err := decimal.NewFromString(raw)
		if err != nil || amount.LessThanOrEqual(decimal.Zero) {
			continue
		}
		f, _ := amount.Float64()
		return f, nil
	}
	return 0, fmt.Errorf("missing amount")
}

func infiniDeterministicRequestID(parts ...string) string {
	hash := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	var id uuid.UUID
	copy(id[:], hash[:16])
	id[6] = (id[6] & 0x0f) | 0x40
	id[8] = (id[8] & 0x3f) | 0x80
	return id.String()
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func summarizeInfiniResponse(body []byte) string {
	summary := strings.Join(strings.Fields(string(body)), " ")
	if summary == "" {
		return "<empty>"
	}
	if len(summary) > infiniMaxErrorSummary {
		return summary[:infiniMaxErrorSummary] + "..."
	}
	return summary
}

type infiniCreateOrderRequest struct {
	Amount          string `json:"amount"`
	RequestID       string `json:"request_id"`
	ClientReference string `json:"client_reference,omitempty"`
	OrderDesc       string `json:"order_desc,omitempty"`
	SuccessURL      string `json:"success_url,omitempty"`
	FailureURL      string `json:"failure_url,omitempty"`
	PayMethods      []int  `json:"pay_methods,omitempty"`
	Currency        string `json:"currency,omitempty"`
}

type infiniCreateOrderResponse struct {
	OrderID         string `json:"order_id"`
	RequestID       string `json:"request_id"`
	CheckoutURL     string `json:"checkout_url"`
	ClientReference string `json:"client_reference"`
}

type infiniOrder struct {
	OrderID         string `json:"order_id"`
	Status          string `json:"status"`
	Amount          string `json:"amount"`
	Currency        string `json:"currency"`
	AmountConfirmed string `json:"amount_confirmed"`
	ClientReference string `json:"client_reference"`
}

type infiniWebhookEvent struct {
	Event           string `json:"event"`
	OrderID         string `json:"order_id"`
	ClientReference string `json:"client_reference"`
	Amount          string `json:"amount"`
	Currency        string `json:"currency"`
	Status          string `json:"status"`
	AmountConfirmed string `json:"amount_confirmed"`
}

var (
	_ payment.Provider                 = (*Infini)(nil)
	_ payment.MerchantIdentityProvider = (*Infini)(nil)
)
