package service

import (
	"context"
	"database/sql"
	"log/slog"
	"sync"
	"time"

	entsql "entgo.io/ent/dialect/sql"
)

const expiryCheckTimeout = 30 * time.Second

// PaymentOrderExpiryService periodically expires timed-out payment orders.
type PaymentOrderExpiryService struct {
	paymentSvc *PaymentService
	interval   time.Duration
	stopCh     chan struct{}
	stopOnce   sync.Once
	wg         sync.WaitGroup
}

func NewPaymentOrderExpiryService(paymentSvc *PaymentService, interval time.Duration) *PaymentOrderExpiryService {
	return &PaymentOrderExpiryService{
		paymentSvc: paymentSvc,
		interval:   interval,
		stopCh:     make(chan struct{}),
	}
}

func (s *PaymentOrderExpiryService) Start() {
	if s == nil || s.paymentSvc == nil || s.interval <= 0 {
		return
	}
	if !s.paymentOrdersTableExists() {
		slog.Warn("[PaymentOrderExpiry] payment_orders table is missing; expiry worker disabled")
		return
	}
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		ticker := time.NewTicker(s.interval)
		defer ticker.Stop()

		s.runOnce()
		for {
			select {
			case <-ticker.C:
				s.runOnce()
			case <-s.stopCh:
				return
			}
		}
	}()
}

func (s *PaymentOrderExpiryService) Stop() {
	if s == nil {
		return
	}
	s.stopOnce.Do(func() {
		close(s.stopCh)
	})
	s.wg.Wait()
}

func (s *PaymentOrderExpiryService) runOnce() {
	ctx, cancel := context.WithTimeout(context.Background(), expiryCheckTimeout)
	defer cancel()

	expired, err := s.paymentSvc.ExpireTimedOutOrders(ctx)
	if err != nil {
		slog.Error("[PaymentOrderExpiry] failed to expire orders", "error", err)
		return
	}
	if expired > 0 {
		slog.Info("[PaymentOrderExpiry] expired timed-out orders", "count", expired)
	}
}

func (s *PaymentOrderExpiryService) paymentOrdersTableExists() bool {
	if s == nil || s.paymentSvc == nil || s.paymentSvc.entClient == nil {
		return false
	}

	ctx, cancel := context.WithTimeout(context.Background(), expiryCheckTimeout)
	defer cancel()

	var rows entsql.Rows
	if err := s.paymentSvc.entClient.Driver().Query(ctx, `SELECT to_regclass('public.payment_orders')`, nil, &rows); err != nil {
		slog.Warn("[PaymentOrderExpiry] failed to probe payment_orders table; worker disabled", "error", err)
		return false
	}
	defer func() { _ = rows.Close() }()

	if !rows.Next() {
		return false
	}

	var tableName sql.NullString
	if err := rows.Scan(&tableName); err != nil {
		slog.Warn("[PaymentOrderExpiry] failed to scan payment_orders probe result; worker disabled", "error", err)
		return false
	}

	return tableName.Valid && tableName.String != ""
}
