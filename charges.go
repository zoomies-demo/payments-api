package main

import (
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"
)

// Charge is one request to take money. Amount is in the currency's smallest
// unit (pence, cents), never a decimal: a float cannot hold 0.1 exactly.
type Charge struct {
	ID        string    `json:"id"`
	Amount    int64     `json:"amount"`
	Currency  string    `json:"currency"`
	Reference string    `json:"reference"`
	CreatedAt time.Time `json:"created_at"`
}

// ChargeRequest is what a caller sends.
type ChargeRequest struct {
	Amount    int64  `json:"amount"`
	Currency  string `json:"currency"`
	Reference string `json:"reference"`
}

var currencies = map[string]bool{"GBP": true, "EUR": true, "USD": true}

// Validate says what is wrong with a request in words a caller can act on.
func (r ChargeRequest) Validate() error {
	switch {
	case r.Amount <= 0:
		return errors.New("amount must be a positive whole number of the currency's smallest unit")
	case r.Amount > 100_000_00:
		return errors.New("amount is above the 100,000.00 limit for a single charge")
	case !currencies[strings.ToUpper(r.Currency)]:
		return fmt.Errorf("currency %q is not one of GBP, EUR or USD", r.Currency)
	case strings.TrimSpace(r.Reference) == "":
		return errors.New("reference is required, so a charge can be found again")
	}
	return nil
}

// Ledger keeps charges in memory, which is all a demo needs. The idempotency
// key is the point of it: a client that retries after a timeout must not take
// the money twice, so the same key returns the charge it made the first time.
type Ledger struct {
	mu     sync.Mutex
	now    func() time.Time
	next   int
	byKey  map[string]Charge
	byID   map[string]Charge
	hits   int
	misses int
}

func NewLedger(now func() time.Time) *Ledger {
	return &Ledger{now: now, byKey: map[string]Charge{}, byID: map[string]Charge{}}
}

// Create makes a charge, or returns the earlier one when the key was seen.
// replay reports which of the two happened.
func (l *Ledger) Create(key string, req ChargeRequest) (c Charge, replay bool, err error) {
	if err := req.Validate(); err != nil {
		return Charge{}, false, err
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if key != "" {
		if prev, ok := l.byKey[key]; ok {
			if prev.Amount != req.Amount || prev.Currency != strings.ToUpper(req.Currency) {
				return Charge{}, false, errors.New("this idempotency key was already used for a different charge")
			}
			l.hits++
			return prev, true, nil
		}
	}
	l.next++
	c = Charge{
		ID:        fmt.Sprintf("ch_%06d", l.next),
		Amount:    req.Amount,
		Currency:  strings.ToUpper(req.Currency),
		Reference: req.Reference,
		CreatedAt: l.now().UTC(),
	}
	l.byID[c.ID] = c
	if key != "" {
		l.byKey[key] = c
	}
	l.misses++
	return c, false, nil
}

// Get returns one charge by ID.
func (l *Ledger) Get(id string) (Charge, bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	c, ok := l.byID[id]
	return c, ok
}
