package runtime

import (
	goruntime "runtime"
	"sort"
	"sync/atomic"

	"kLang/src/diagnostic"
	"kLang/src/parser"
)

// The STM protocol follows the versioned-read and commit-time locking shape of
// TL2: https://people.csail.mit.edu/shanir/publications/Transactional_Locking.pdf
// Flattened nesting follows the composability goal described by Harris et al.:
// https://cs.brown.edu/~mph/HarrisMPJH05/stm.pdf
// Atomic values remain individually linearizable outside a transaction.

type atomicUint64 struct {
	value atomic.Uint64
}

func (value *atomicUint64) Load() uint64 {
	return value.value.Load()
}

func (value *atomicUint64) Store(next uint64) {
	value.value.Store(next)
}

var stmClock atomic.Uint64
var stmAtomicID atomic.Uint64

const transactionRetryLimit = 1024

type transactionConflict struct{}

func (transactionConflict) Error() string {
	return "transaction conflict"
}

type transactionContext struct {
	readVersion uint64
	reads       map[*AtomicData]uint64
	writes      map[*AtomicData]Value
}

func newAtomicValue(value Value) Value {
	return Value{
		Kind: ValueAtomic,
		Data: &AtomicData{
			Value: value,
			ID:    stmAtomicID.Add(1),
		},
	}
}

func newTransactionContext() *transactionContext {
	return &transactionContext{
		readVersion: stmClock.Load(),
		reads:       map[*AtomicData]uint64{},
		writes:      map[*AtomicData]Value{},
	}
}

func (transaction *transactionContext) load(cell *AtomicData) (Value, error) {
	if value, ok := transaction.writes[cell]; ok {
		return cloneValue(value), nil
	}

	cell.Mutex.Lock()
	version := cell.Version.Load()
	value := cloneValue(cell.Value)
	cell.Mutex.Unlock()

	if version > transaction.readVersion {
		return NullValue(), transactionConflict{}
	}
	if expected, exists := transaction.reads[cell]; exists && expected != version {
		return NullValue(), transactionConflict{}
	}
	transaction.reads[cell] = version
	if !transaction.valid() {
		return NullValue(), transactionConflict{}
	}
	return value, nil
}

func (transaction *transactionContext) store(cell *AtomicData, value Value) {
	if _, exists := transaction.reads[cell]; !exists {
		transaction.reads[cell] = cell.Version.Load()
	}
	transaction.writes[cell] = cloneValue(value)
}

func (transaction *transactionContext) valid() bool {
	for cell, expected := range transaction.reads {
		if cell.Version.Load() != expected {
			return false
		}
	}
	return true
}

func (transaction *transactionContext) commit() bool {
	cells := make([]*AtomicData, 0, len(transaction.writes))
	for cell := range transaction.writes {
		cells = append(cells, cell)
	}
	sort.Slice(cells, func(left, right int) bool {
		return cells[left].ID < cells[right].ID
	})
	for _, cell := range cells {
		cell.Mutex.Lock()
	}
	defer func() {
		for index := len(cells) - 1; index >= 0; index-- {
			cells[index].Mutex.Unlock()
		}
	}()

	if !transaction.valid() {
		return false
	}
	if len(cells) == 0 {
		return true
	}
	version := stmClock.Add(1)
	for _, cell := range cells {
		cell.Value = cloneValue(transaction.writes[cell])
		cell.Version.Store(version)
	}
	return true
}

func (runtime *Runtime) executeTransaction(stmt parser.TransactionStatement, env *Environment, inLoop bool) (signal, error) {
	if runtime.transaction != nil {
		return runtime.executeBlock(stmt.Body, NewEnvironment(env), inLoop)
	}

	for attempt := 0; attempt < transactionRetryLimit; attempt++ {
		transaction := newTransactionContext()
		runtime.transaction = transaction
		currentSignal, err := runtime.executeBlock(stmt.Body, NewEnvironment(env), inLoop)
		runtime.transaction = nil

		if _, conflict := err.(transactionConflict); conflict {
			goruntime.Gosched()
			continue
		}
		if err != nil || currentSignal.kind != signalNone {
			if !transaction.valid() {
				goruntime.Gosched()
				continue
			}
			return currentSignal, err
		}
		if transaction.commit() {
			return signal{kind: signalNone}, nil
		}
		goruntime.Gosched()
	}
	return signal{}, errorAtCode(
		stmt.Pos,
		diagnostic.CodeTransactionConflict,
		"transaction progress",
		"transaction retry limit exceeded",
		"Reduce contention, shorten the transaction body, or retry the operation at a higher level.",
	)
}
