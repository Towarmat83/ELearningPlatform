// Package fake provides in-memory fakes of the db.Pool interface and its
// related types, so handler tests can control query results without a real
// database connection.
package fake

import (
	"context"
	"errors"
	"fmt"
	"reflect"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/genesary/pupitre/user-service/internal/db"
)

// Pool is a fake db.Pool for testing.
type Pool struct {
	rowQueue  []*Row
	rowsQueue []*Rows
	execQueue []execResult
}

// execResult is a queued response for an Exec call.
type execResult struct {
	tag pgconn.CommandTag
	err error
}

// PushRow queues a QueryRow response.
func (p *Pool) PushRow(err error, vals ...any) {
	p.rowQueue = append(p.rowQueue, &Row{vals: vals, err: err})
}

// PushRows queues a Query response.
func (p *Pool) PushRows(err error, rows ...[]any) {
	p.rowsQueue = append(p.rowsQueue, &Rows{data: rows, err: err})
}

// PushExec queues an Exec response.
func (p *Pool) PushExec(rowsAffected int64, err error) {
	tag := pgconn.NewCommandTag(fmt.Sprintf("UPDATE %d", rowsAffected))
	p.execQueue = append(p.execQueue, execResult{tag: tag, err: err})
}

// QueryRow returns the next queued row response, or an error-carrying Row
// if none is queued.
//
//nolint:ireturn // return type is pinned by the db.Pool interface it fakes
func (p *Pool) QueryRow(_ context.Context, _ string, _ ...any) db.Row {
	if len(p.rowQueue) == 0 {
		return &Row{err: errors.New("fake pool: no QueryRow response queued")}
	}

	queued := p.rowQueue[0]
	p.rowQueue = p.rowQueue[1:]

	return queued
}

// Query returns the next queued rows response, or an error if none is
// queued.
//
//nolint:ireturn // return type is pinned by the db.Pool interface it fakes
func (p *Pool) Query(_ context.Context, _ string, _ ...any) (pgx.Rows, error) {
	if len(p.rowsQueue) == 0 {
		return nil, errors.New("fake pool: no Query response queued")
	}

	queued := p.rowsQueue[0]
	p.rowsQueue = p.rowsQueue[1:]

	if queued.err != nil {
		return nil, queued.err
	}

	return queued, nil
}

// Exec returns the next queued exec response, or an OK tag with zero rows
// affected if none is queued.
func (p *Pool) Exec(_ context.Context, _ string, _ ...any) (pgconn.CommandTag, error) {
	if len(p.execQueue) == 0 {
		return pgconn.NewCommandTag("OK"), nil
	}

	queued := p.execQueue[0]
	p.execQueue = p.execQueue[1:]

	return queued.tag, queued.err
}

// Row implements db.Row.
type Row struct {
	vals []any
	err  error
}

// Scan copies the queued values into dest, or returns the queued error.
func (r *Row) Scan(dest ...any) error {
	if r.err != nil {
		return r.err
	}

	return assignAll(dest, r.vals)
}

// Rows implements pgx.Rows.
type Rows struct {
	data [][]any
	idx  int
	err  error
}

// Next advances to the next queued row and reports whether one is
// available.
func (r *Rows) Next() bool {
	r.idx++

	return r.idx <= len(r.data)
}

// Close is a no-op; the fake Rows holds no resources to release.
func (r *Rows) Close() {}

// Err returns the error associated with iteration, if any.
func (r *Rows) Err() error { return r.err }

// CommandTag returns a zero-value CommandTag; the fake does not track one.
func (r *Rows) CommandTag() pgconn.CommandTag { return pgconn.CommandTag{} }

// FieldDescriptions returns nil; the fake does not track field metadata.
func (r *Rows) FieldDescriptions() []pgconn.FieldDescription { return nil }

// Values returns nil, nil; the fake does not support raw value extraction.
func (r *Rows) Values() ([]any, error) { return nil, nil }

// RawValues returns nil; the fake does not track raw byte values.
func (r *Rows) RawValues() [][]byte { return nil }

// Conn returns nil; the fake is not backed by a real connection.
func (r *Rows) Conn() *pgx.Conn { return nil }

// Scan copies the current row's values into dest, or returns an error if
// Next has not been called yet.
func (r *Rows) Scan(dest ...any) error {
	if r.idx <= 0 || r.idx > len(r.data) {
		return errors.New("no current row")
	}

	return assignAll(dest, r.data[r.idx-1])
}

// assignAll assigns values to destination pointers.
func assignAll(dests, vals []any) error {
	for idx, dst := range dests {
		var val any
		if idx < len(vals) {
			val = vals[idx]
		}

		err := assignOne(dst, val)
		if err != nil {
			return fmt.Errorf("col %d: %w", idx, err)
		}
	}

	return nil
}

// assignOne assigns src into the value pointed to by dst, performing the
// same kind of type conversions a real database driver's Scan would apply.
func assignOne(dst, src any) error {
	if src == nil {
		return nil
	}

	dstVal := reflect.ValueOf(dst)
	if dstVal.Kind() != reflect.Pointer {
		return fmt.Errorf("dst must be pointer, got %T", dst)
	}

	elem := dstVal.Elem()
	srcVal := reflect.ValueOf(src)

	if assignDirect(elem, srcVal) {
		return nil
	}

	if elem.Kind() == reflect.Pointer && assignThroughPointer(elem, srcVal) {
		return nil
	}

	if assignNumeric(elem, srcVal) {
		return nil
	}

	if srcVal.CanConvert(elem.Type()) {
		elem.Set(srcVal.Convert(elem.Type()))

		return nil
	}

	return fmt.Errorf("cannot assign %T to %T", src, dst)
}

// assignDirect assigns src into elem when their types are identical.
func assignDirect(elem, src reflect.Value) bool {
	if elem.Type() != src.Type() {
		return false
	}

	elem.Set(src)

	return true
}

// assignThroughPointer allocates a new value of elem's pointee type and
// assigns src into it when elem is a pointer and src is compatible with the
// pointee type.
func assignThroughPointer(elem, src reflect.Value) bool {
	inner := elem.Type().Elem()
	if src.Type() != inner && !src.Type().AssignableTo(inner) {
		return false
	}

	ptr := reflect.New(inner)
	ptr.Elem().Set(src)
	elem.Set(ptr)

	return true
}

// assignNumeric assigns src into elem when both are compatible integer or
// float kinds.
func assignNumeric(elem, src reflect.Value) bool {
	switch {
	case elem.CanInt() && src.CanInt():
		elem.SetInt(src.Int())

		return true
	case elem.CanFloat() && src.CanFloat():
		elem.SetFloat(src.Float())

		return true
	default:
		return false
	}
}
