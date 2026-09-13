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
	cfg["currency"] = payment.InfiniSettlementCurrency(cfg["currency"])
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
	return payment.InfiniSettlementCurrency(i.config["currency"])
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
		Amount:          amount.StringFixed(int32(payment.CurrencyMaxFractionDigits(currency))),
		RequestID:       infiniDeterministicRequestID("order", req.OrderID, req.Amount, currency),
		ClientReference: req.OrderID,
		OrderDesc:       strings.TrimSpace(req.Subject),
		SuccessURL:      strings.TrimSpace(req.ReturnURL),
		FailureURL:      strings.TrimSpace(req.ReturnURL),
		PayMethods:      payMethods,
		Currency:        currency,
	}

	raw, err := i.doJSONBytes(ctx, http.MethodPost, "/v1/acquiring/order", payload)
	if err != nil {
		return nil, fmt.Errorf("infini create payment: %w", err)
	}
	resp, err := parseInfiniCreateOrderResponse(raw)
	if err != nil {
		return nil, fmt.Errorf("infini create payment: %w", err)
	}
	orderID := resp.orderID()
	checkoutURL := resp.checkoutURL()
	if orderID != "" && checkoutURL == "" {
		if reissued, reissueErr := i.reissueCheckoutURL(ctx, orderID); reissueErr == nil {
			checkoutURL = reissued
		}
	}
	if orderID == "" || checkoutURL == "" {
		return nil, fmt.Errorf("infini create payment: missing order_id or checkout_url: %s", summarizeInfiniResponse(raw))
	}
	return &payment.CreatePaymentResponse{
		TradeNo:    orderID,
		PayURL:     checkoutURL,
		Currency:   currency,
		PaymentEnv: i.checkoutEnv(),
		ResultType: payment.CreatePaymentResultOrderCreated,
	}, nil
}

func (i *Infini) reissueCheckoutURL(ctx context.Context, orderID string) (string, error) {
	raw, err := i.doJSONBytes(ctx, http.MethodPost, "/v1/acquiring/token/reissue", map[string]string{"order_id": orderID})
	if err != nil {
		return "", err
	}
	resp, err := parseInfiniCreateOrderResponse(raw)
	if err != nil {
		return "", err
	}
	if url := resp.checkoutURL(); url != "" {
		return url, nil
	}
	return "", fmt.Errorf("reissue missing checkout_url: %s", summarizeInfiniResponse(raw))
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
	if strings.TrimSpace(resp.OrderID) == "" {
		resp.OrderID = orderID
	}
	confirmedExact, confirmed, confirmedErr := parseInfiniAmount(resp.AmountConfirmed)
	listedExact, listed, listedErr := parseInfiniAmount(resp.Amount)
	status, amountExact, amount, err := infiniQueryDecision(resp.Status, confirmedExact, confirmed, confirmedErr, listedExact, listed, listedErr)
	if err != nil {
		return nil, err
	}
	return &payment.QueryOrderResponse{
		TradeNo:  firstNonEmpty(resp.OrderID, orderID),
		Status:   status,
		Amount:   amount,
		Metadata: i.orderMetadata(resp, amountExact),
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

	status, amountExact, amount, err := infiniWebhookDecision(event)
	if err != nil {
		return nil, err
	}
	if status == "" {
		return nil, nil
	}

	return &payment.PaymentNotification{
		TradeNo:     tradeNo,
		OrderID:     orderID,
		Amount:      amount,
		AmountExact: amountExact,
		EventID:     strings.TrimSpace(headerCI(headers, "x-webhook-event-id")),
		Status:      status,
		RawData:     rawBody,
		Metadata:    i.eventMetadata(event, amountExact),
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

func (i *Infini) orderMetadata(order infiniOrder, amountExact string) map[string]string {
	return map[string]string{
		"currency":          strings.ToUpper(strings.TrimSpace(order.Currency)),
		"status":            strings.ToLower(strings.TrimSpace(order.Status)),
		"amount_exact":      amountExact,
		"client_reference":  strings.TrimSpace(order.ClientReference),
		"provider_order_id": strings.TrimSpace(order.OrderID),
	}
}

func (i *Infini) eventMetadata(event infiniWebhookEvent, amountExact string) map[string]string {
	return map[string]string{
		"currency":          strings.ToUpper(strings.TrimSpace(event.Currency)),
		"status":            strings.ToLower(strings.TrimSpace(event.Status)),
		"event":             strings.ToLower(strings.TrimSpace(event.Event)),
		"amount_exact":      amountExact,
		"client_reference":  strings.TrimSpace(event.ClientReference),
		"provider_order_id": strings.TrimSpace(event.OrderID),
	}
}

func (i *Infini) doJSON(ctx context.Context, method, path string, payload any, out any) error {
	respBody, err := i.doJSONBytes(ctx, method, path, payload)
	if err != nil {
		return err
	}
	if out == nil || len(bytes.TrimSpace(respBody)) == 0 {
		return nil
	}
	payloadBody, err := unwrapInfiniPayload(respBody)
	if err != nil {
		return err
	}
	if len(bytes.TrimSpace(payloadBody)) == 0 {
		return nil
	}
	if err := json.Unmarshal(payloadBody, out); err != nil {
		return fmt.Errorf("parse response: %w; body=%s", err, summarizeInfiniResponse(respBody))
	}
	return nil
}

func (i *Infini) doJSONBytes(ctx context.Context, method, path string, payload any) ([]byte, error) {
	var bodyBytes []byte
	if payload != nil {
		b, err := json.Marshal(payload)
		if err != nil {
			return nil, err
		}
		bodyBytes = b
	}
	headers, err := infiniSignRequest(i.config["keyId"], i.config["secretKey"], method, path, bodyBytes, i.now)
	if err != nil {
		return nil, err
	}

	var body io.Reader
	if len(bodyBytes) > 0 {
		body = bytes.NewReader(bodyBytes)
		headers["Content-Type"] = "application/json"
	}
	req, err := http.NewRequestWithContext(ctx, method, i.config["apiBase"]+path, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	client := i.httpClient
	if client == nil {
		client = &http.Client{Timeout: infiniHTTPTimeout}
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	respBody, err := io.ReadAll(io.LimitReader(resp.Body, infiniMaxResponseSize))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		if unwrapped, unwrapErr := unwrapInfiniPayload(respBody); unwrapErr != nil {
			return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, unwrapErr.Error())
		} else if msg := infiniEnvelopeMessage(unwrapped); msg != "" {
			return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, msg)
		}
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, summarizeInfiniResponse(respBody))
	}
	if _, err := unwrapInfiniPayload(respBody); err != nil {
		return nil, err
	}
	return respBody, nil
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

func infiniWebhookDecision(event infiniWebhookEvent) (status, amountExact string, amount float64, err error) {
	eventName := strings.ToLower(strings.TrimSpace(event.Event))
	eventStatus := strings.ToLower(strings.TrimSpace(event.Status))
	confirmedExact, confirmed, confirmedErr := parseInfiniAmount(event.AmountConfirmed)
	listedExact, listed, listedErr := parseInfiniAmount(event.Amount)

	switch eventName {
	case infiniEventCompleted:
		if eventStatus != infiniStatusPaid {
			return "", "", 0, nil
		}
		if confirmedErr == nil {
			return payment.NotificationStatusSuccess, confirmedExact, confirmed, nil
		}
		if listedErr == nil {
			return payment.NotificationStatusSuccess, listedExact, listed, nil
		}
		return "", "", 0, fmt.Errorf("infini webhook invalid amount: missing amount_confirmed")
	case infiniEventLatePayment:
		if confirmedErr != nil {
			return "", "", 0, fmt.Errorf("infini late_payment missing amount_confirmed")
		}
		if eventStatus != "" && eventStatus != infiniStatusPaid && eventStatus != infiniStatusExpired {
			return "", "", 0, fmt.Errorf("infini late_payment has non-payable status %s", eventStatus)
		}
		// Official late_payment keeps status=expired; amount_confirmed is the
		// settled amount and may be partial. Fulfillment still exact-matches
		// the order snapshot and rejects under/over pay.
		return payment.NotificationStatusSuccess, confirmedExact, confirmed, nil
	case infiniEventExpired:
		if confirmedErr == nil {
			return payment.NotificationStatusSuccess, confirmedExact, confirmed, nil
		}
		return payment.ProviderStatusFailed, listedExact, listed, nil
	default:
		return "", "", 0, nil
	}
}

func infiniQueryDecision(status, confirmedExact string, confirmed float64, confirmedErr error, listedExact string, listed float64, listedErr error) (string, string, float64, error) {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case infiniStatusPaid:
		if confirmedErr == nil {
			return payment.ProviderStatusPaid, confirmedExact, confirmed, nil
		}
		if listedErr == nil {
			return payment.ProviderStatusPaid, listedExact, listed, nil
		}
		return "", "", 0, fmt.Errorf("infini query order: missing confirmed amount")
	case infiniStatusExpired:
		// Late payment stays expired; only amount_confirmed may prove settlement.
		// Never fall back to the original payable `amount`.
		if confirmedErr == nil {
			return payment.ProviderStatusPaid, confirmedExact, confirmed, nil
		}
		return payment.ProviderStatusFailed, listedExact, listed, nil
	case infiniStatusPartialPaid:
		return payment.ProviderStatusFailed, firstNonEmpty(confirmedExact, listedExact), confirmedOrListed(confirmed, listed, confirmedErr), nil
	default:
		return payment.ProviderStatusPending, firstNonEmpty(confirmedExact, listedExact), confirmedOrListed(confirmed, listed, confirmedErr), nil
	}
}

func confirmedOrListed(confirmed, listed float64, confirmedErr error) float64 {
	if confirmedErr == nil {
		return confirmed
	}
	return listed
}

func parseInfiniAmount(values ...string) (string, float64, error) {
	for _, raw := range values {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}
		amount, err := decimal.NewFromString(raw)
		if err != nil || amount.LessThanOrEqual(decimal.Zero) {
			continue
		}
		exact := amount.String()
		f, _ := amount.Float64()
		return exact, f, nil
	}
	return "", 0, fmt.Errorf("missing amount")
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

func unwrapInfiniPayload(body []byte) ([]byte, error) {
	trimmed := bytes.TrimSpace(body)
	if len(trimmed) == 0 || trimmed[0] != '{' {
		return trimmed, nil
	}
	var env infiniAPIEnvelope
	if err := json.Unmarshal(trimmed, &env); err != nil {
		return trimmed, nil
	}
	if env.Success != nil && !*env.Success {
		return nil, fmt.Errorf("%s", firstNonEmpty(env.Error, env.Message, env.Detail, "infini request failed"))
	}
	if code, ok := parseInfiniEnvelopeCode(env.Code); ok && !infiniEnvelopeCodeOK(code) {
		return nil, fmt.Errorf("infini error %s: %s", code, firstNonEmpty(env.Message, env.Detail, env.Error, "request failed"))
	}
	if payload := nonemptyJSONObject(env.Data); len(payload) > 0 {
		return payload, nil
	}
	if payload := nonemptyJSONObject(env.Result); len(payload) > 0 {
		return payload, nil
	}
	return trimmed, nil
}

func parseInfiniCreateOrderResponse(body []byte) (infiniCreateOrderResponse, error) {
	payload, err := unwrapInfiniPayload(body)
	if err != nil {
		return infiniCreateOrderResponse{}, err
	}
	var resp infiniCreateOrderResponse
	if len(bytes.TrimSpace(payload)) > 0 {
		_ = json.Unmarshal(payload, &resp)
	}
	if resp.orderID() == "" || resp.checkoutURL() == "" {
		if found := infiniCreateOrderFromMap(payload); found.orderID() != "" || found.checkoutURL() != "" {
			resp = mergeInfiniCreateOrder(resp, found)
		}
	}
	return resp, nil
}

func infiniCreateOrderFromMap(payload []byte) infiniCreateOrderResponse {
	var raw map[string]any
	if err := json.Unmarshal(payload, &raw); err != nil || raw == nil {
		return infiniCreateOrderResponse{}
	}
	found := pickInfiniCreateOrder(raw)
	if found.orderID() != "" && found.checkoutURL() != "" {
		return found
	}
	for _, value := range raw {
		child, ok := value.(map[string]any)
		if !ok {
			continue
		}
		nested := pickInfiniCreateOrder(child)
		if nested.orderID() != "" || nested.checkoutURL() != "" {
			return mergeInfiniCreateOrder(found, nested)
		}
	}
	return found
}

func pickInfiniCreateOrder(raw map[string]any) infiniCreateOrderResponse {
	return infiniCreateOrderResponse{
		OrderID:     infiniFlexibleString(firstNonEmpty(jsonAnyString(raw["order_id"]), jsonAnyString(raw["orderId"]), jsonAnyString(raw["id"]))),
		CheckoutURL: firstNonEmpty(jsonAnyString(raw["checkout_url"]), jsonAnyString(raw["checkoutUrl"]), jsonAnyString(raw["pay_url"]), jsonAnyString(raw["payUrl"]), jsonAnyString(raw["url"]), jsonAnyString(raw["link"])),
		Token:       jsonAnyString(raw["token"]),
	}
}

func mergeInfiniCreateOrder(base, extra infiniCreateOrderResponse) infiniCreateOrderResponse {
	if base.orderID() == "" {
		base.OrderID = extra.OrderID
		base.OrderIDCamel = extra.OrderIDCamel
		base.ID = extra.ID
	}
	if base.checkoutURL() == "" {
		base.CheckoutURL = extra.CheckoutURL
		base.CheckoutURLCamel = extra.CheckoutURLCamel
		base.PayURL = extra.PayURL
		base.URL = extra.URL
		base.Link = extra.Link
		base.Token = extra.Token
	}
	return base
}

func parseInfiniEnvelopeCode(raw json.RawMessage) (string, bool) {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || string(raw) == "null" {
		return "", false
	}
	var asNumber json.Number
	if err := json.Unmarshal(raw, &asNumber); err == nil {
		return asNumber.String(), true
	}
	var asString string
	if err := json.Unmarshal(raw, &asString); err == nil {
		asString = strings.TrimSpace(asString)
		if asString == "" {
			return "", false
		}
		return asString, true
	}
	return strings.TrimSpace(string(raw)), true
}

func infiniEnvelopeCodeOK(code string) bool {
	switch strings.ToLower(strings.TrimSpace(code)) {
	case "", "0", "200", "ok", "success", "succeeded":
		return true
	default:
		return false
	}
}

func nonemptyJSONObject(raw json.RawMessage) []byte {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || string(raw) == "null" {
		return nil
	}
	if raw[0] == '{' || raw[0] == '[' {
		return raw
	}
	var encoded string
	if err := json.Unmarshal(raw, &encoded); err == nil {
		encoded = strings.TrimSpace(encoded)
		if strings.HasPrefix(encoded, "{") || strings.HasPrefix(encoded, "[") {
			return []byte(encoded)
		}
	}
	return nil
}

func infiniEnvelopeMessage(body []byte) string {
	var env infiniAPIEnvelope
	if err := json.Unmarshal(bytes.TrimSpace(body), &env); err != nil {
		return ""
	}
	return firstNonEmpty(env.Message, env.Detail, env.Error)
}

func jsonAnyString(value any) string {
	switch typed := value.(type) {
	case nil:
		return ""
	case string:
		return strings.TrimSpace(typed)
	case json.Number:
		return strings.TrimSpace(typed.String())
	case float64:
		if typed == 0 {
			return ""
		}
		return strconv.FormatInt(int64(typed), 10)
	default:
		return strings.TrimSpace(fmt.Sprint(typed))
	}
}

type infiniFlexibleString string

func (s *infiniFlexibleString) UnmarshalJSON(raw []byte) error {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || string(raw) == "null" {
		*s = ""
		return nil
	}
	if raw[0] == '"' {
		var text string
		if err := json.Unmarshal(raw, &text); err != nil {
			return err
		}
		*s = infiniFlexibleString(strings.TrimSpace(text))
		return nil
	}
	*s = infiniFlexibleString(strings.Trim(string(raw), `"`))
	return nil
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

type infiniAPIEnvelope struct {
	Code    json.RawMessage `json:"code"`
	Success *bool           `json:"success"`
	Message string          `json:"message"`
	Detail  string          `json:"detail"`
	Error   string          `json:"error"`
	Data    json.RawMessage `json:"data"`
	Result  json.RawMessage `json:"result"`
}

type infiniCreateOrderResponse struct {
	OrderID          infiniFlexibleString `json:"order_id"`
	OrderIDCamel     infiniFlexibleString `json:"orderId"`
	ID               infiniFlexibleString `json:"id"`
	CheckoutURL      string               `json:"checkout_url"`
	CheckoutURLCamel string               `json:"checkoutUrl"`
	PayURL           string               `json:"pay_url"`
	URL              string               `json:"url"`
	Link             string               `json:"link"`
	Token            string               `json:"token"`
}

func (r infiniCreateOrderResponse) orderID() string {
	return firstNonEmpty(string(r.OrderID), string(r.OrderIDCamel), string(r.ID))
}

func (r infiniCreateOrderResponse) checkoutURL() string {
	url := firstNonEmpty(r.CheckoutURL, r.CheckoutURLCamel, r.PayURL, r.URL, r.Link)
	if url != "" {
		return url
	}
	if strings.HasPrefix(r.Token, "https://") || strings.HasPrefix(r.Token, "http://") {
		return r.Token
	}
	return ""
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
