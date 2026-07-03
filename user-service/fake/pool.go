package fake

import (
	"context"
	"errors"
	"fmt"
	"reflect"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/elearning/user-service/internal/db"
)

// Pool is a fake db.Pool for testing.
type Pool struct {
	rowQueue  []*Row
	rowsQueue []*Rows
	execQueue []execResult
}

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

func (p *Pool) QueryRow(_ context.Context, _ string, _ ...any) db.Row {
	if len(p.rowQueue) == 0 {
		return &Row{err: errors.New("fake pool: no QueryRow response queued")}
	}

	r := p.rowQueue[0]
	p.rowQueue = p.rowQueue[1:]

	return r
}

func (p *Pool) Query(_ context.Context, _ string, _ ...any) (pgx.Rows, error) {
	if len(p.rowsQueue) == 0 {
		return nil, errors.New("fake pool: no Query response queued")
	}

	r := p.rowsQueue[0]
	p.rowsQueue = p.rowsQueue[1:]

	if r.err != nil {
		return nil, r.err
	}

	return r, nil
}

func (p *Pool) Exec(_ context.Context, _ string, _ ...any) (pgconn.CommandTag, error) {
	if len(p.execQueue) == 0 {
		return pgconn.NewCommandTag("OK"), nil
	}

	r := p.execQueue[0]
	p.execQueue = p.execQueue[1:]

	return r.tag, r.err
}

// Row implements db.Row.
type Row struct {
	vals []any
	err  error
}

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

func (r *Rows) Next() bool {
	r.idx++

	return r.idx <= len(r.data)
}
func (r *Rows) Close()                                       {}
func (r *Rows) Err() error                                   { return r.err }
func (r *Rows) CommandTag() pgconn.CommandTag                { return pgconn.CommandTag{} }
func (r *Rows) FieldDescriptions() []pgconn.FieldDescription { return nil }
func (r *Rows) Values() ([]any, error)                       { return nil, nil }
func (r *Rows) RawValues() [][]byte                          { return nil }
func (r *Rows) Conn() *pgx.Conn                              { return nil }

func (r *Rows) Scan(dest ...any) error {
	if r.idx <= 0 || r.idx > len(r.data) {
		return errors.New("no current row")
	}

	return assignAll(dest, r.data[r.idx-1])
}

// assignAll assigns values to destination pointers.
func assignAll(dests, vals []any) error {
	for i, dst := range dests {
		var val any
		if i < len(vals) {
			val = vals[i]
		}

		err := assignOne(dst, val)
		if err != nil {
			return fmt.Errorf("col %d: %w", i, err)
		}
	}

	return nil
}

func assignOne(dst, src any) error {
	if src == nil {
		return nil
	}

	dv := reflect.ValueOf(dst)
	if dv.Kind() != reflect.Pointer {
		return fmt.Errorf("dst must be pointer, got %T", dst)
	}

	de := dv.Elem()
	sv := reflect.ValueOf(src)

	if de.Type() == sv.Type() {
		de.Set(sv)

		return nil
	}

	if de.Kind() == reflect.Pointer {
		inner := de.Type().Elem()
		if sv.Type() == inner || sv.Type().AssignableTo(inner) {
			ptr := reflect.New(inner)
			ptr.Elem().Set(sv)
			de.Set(ptr)

			return nil
		}
	}

	if de.CanInt() && sv.CanInt() {
		de.SetInt(sv.Int())

		return nil
	}

	if de.CanFloat() && sv.CanFloat() {
		de.SetFloat(sv.Float())

		return nil
	}

	if sv.CanConvert(de.Type()) {
		de.Set(sv.Convert(de.Type()))

		return nil
	}

	return fmt.Errorf("cannot assign %T to %T", src, dst)
}
