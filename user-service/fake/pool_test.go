package fake

import (
	"errors"
	"testing"
)

// TestPushRow_And_QueryRow_Scan verifies that a queued row is returned by
// QueryRow and scanned into the destination pointers.
func TestPushRow_And_QueryRow_Scan(t *testing.T) {
	t.Parallel()

	ctx := t.Context()

	p := &Pool{}
	p.PushRow(nil, "hello", int64(42))

	row := p.QueryRow(ctx, "SELECT 1")

	var (
		s string
		n int64
	)

	err := row.Scan(&s, &n)
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}

	if s != "hello" {
		t.Errorf("string: want hello, got %q", s)
	}

	if n != 42 {
		t.Errorf("int64: want 42, got %d", n)
	}
}

// TestPushRow_Error verifies that a queued error is returned from Scan.
func TestPushRow_Error(t *testing.T) {
	t.Parallel()

	ctx := t.Context()

	p := &Pool{}
	p.PushRow(errors.New("scan error"))

	row := p.QueryRow(ctx, "SELECT 1")

	err := row.Scan()
	if err == nil {
		t.Error("expected scan error, got nil")
	}
}

// TestQueryRow_EmptyQueue verifies that QueryRow returns an error when no
// response has been queued.
func TestQueryRow_EmptyQueue(t *testing.T) {
	t.Parallel()

	ctx := t.Context()

	p := &Pool{}

	row := p.QueryRow(ctx, "SELECT 1")

	err := row.Scan()
	if err == nil {
		t.Error("expected error from empty queue")
	}
}

// TestPushRows_And_Query verifies that queued rows are iterated and scanned
// correctly via the returned pgx.Rows.
func TestPushRows_And_Query(t *testing.T) {
	t.Parallel()

	ctx := t.Context()

	p := &Pool{}
	p.PushRows(nil,
		[]any{"row1", int64(1)},
		[]any{"row2", int64(2)},
	)

	rows, err := p.Query(ctx, "SELECT 1")
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	defer rows.Close()

	count := 0

	for rows.Next() {
		var (
			s string
			n int64
		)

		err := rows.Scan(&s, &n)
		if err != nil {
			t.Errorf("Scan row %d: %v", count, err)
		}

		count++
	}

	if count != 2 {
		t.Errorf("expected 2 rows, got %d", count)
	}

	if rows.Err() != nil {
		t.Errorf("rows.Err(): %v", rows.Err())
	}
}

// TestPushRows_Error verifies that a queued error is returned from Query.
func TestPushRows_Error(t *testing.T) {
	t.Parallel()

	ctx := t.Context()

	p := &Pool{}
	p.PushRows(errors.New("query error"))

	rows, err := p.Query(ctx, "SELECT 1")
	if rows != nil {
		defer rows.Close()
	}

	if err == nil {
		t.Error("expected query error, got nil")
	}
}

// TestQuery_EmptyQueue verifies that Query returns an error when no response
// has been queued.
func TestQuery_EmptyQueue(t *testing.T) {
	t.Parallel()

	ctx := t.Context()

	p := &Pool{}

	rows, err := p.Query(ctx, "SELECT 1")
	if rows != nil {
		defer rows.Close()
	}

	if err == nil {
		t.Error("expected error from empty queue")
	}
}

// TestPushExec_RowsAffected verifies that a queued exec result reports the
// expected number of affected rows.
func TestPushExec_RowsAffected(t *testing.T) {
	t.Parallel()

	ctx := t.Context()

	p := &Pool{}
	p.PushExec(5, nil)

	tag, err := p.Exec(ctx, "UPDATE foo SET x=1")
	if err != nil {
		t.Fatalf("Exec: %v", err)
	}

	if tag.RowsAffected() != 5 {
		t.Errorf("RowsAffected: want 5, got %d", tag.RowsAffected())
	}
}

// TestPushExec_Error verifies that a queued error is returned from Exec.
func TestPushExec_Error(t *testing.T) {
	t.Parallel()

	ctx := t.Context()

	p := &Pool{}
	p.PushExec(0, errors.New("exec error"))

	_, err := p.Exec(ctx, "DELETE FROM foo")
	if err == nil {
		t.Error("expected exec error, got nil")
	}
}

// TestExec_EmptyQueue_ReturnsOK verifies that Exec returns a zero-affected
// OK tag when no response has been queued.
func TestExec_EmptyQueue_ReturnsOK(t *testing.T) {
	t.Parallel()

	ctx := t.Context()

	p := &Pool{}

	tag, err := p.Exec(ctx, "INSERT INTO foo VALUES (1)")
	if err != nil {
		t.Fatalf("empty Exec: %v", err)
	}

	if tag.RowsAffected() != 0 {
		t.Errorf("expected 0 rows affected for OK tag, got %d", tag.RowsAffected())
	}
}

// TestAssignOne_PointerToString verifies that a string source can be
// assigned into a pointer-to-string destination.
func TestAssignOne_PointerToString(t *testing.T) {
	t.Parallel()

	var dst *string

	err := assignOne(&dst, "hello")
	if err != nil {
		t.Fatalf("assignOne: %v", err)
	}

	if dst == nil || *dst != "hello" {
		t.Errorf("expected *string pointing to 'hello', got %v", dst)
	}
}

// TestAssignOne_NilSource verifies that a nil source leaves the destination
// at its zero value.
func TestAssignOne_NilSource(t *testing.T) {
	t.Parallel()

	var dst string

	err := assignOne(&dst, nil)
	if err != nil {
		t.Fatalf("assignOne nil: %v", err)
	}
	// dst should remain unchanged (zero value)
	if dst != "" {
		t.Errorf("expected empty, got %q", dst)
	}
}

// TestAssignOne_IntToInt64 verifies that an int64 source is assigned as-is
// into an int64 destination.
func TestAssignOne_IntToInt64(t *testing.T) {
	t.Parallel()

	var dst int64

	err := assignOne(&dst, int64(100))
	if err != nil {
		t.Fatalf("assignOne int64: %v", err)
	}

	if dst != 100 {
		t.Errorf("expected 100, got %d", dst)
	}
}

// TestAssignOne_IntConversion verifies that an int source is converted into
// an int64 destination.
func TestAssignOne_IntConversion(t *testing.T) {
	t.Parallel()

	var dst int64

	err := assignOne(&dst, 42)
	if err != nil {
		t.Fatalf("assignOne int: %v", err)
	}

	if dst != 42 {
		t.Errorf("expected 42, got %d", dst)
	}
}

// TestAssignAll_MismatchedLengths verifies that extra destinations beyond
// the number of provided values are left untouched.
func TestAssignAll_MismatchedLengths(t *testing.T) {
	t.Parallel()

	dests := make([]any, 3)

	var s1, s2, s3 string

	dests[0] = &s1
	dests[1] = &s2
	dests[2] = &s3

	vals := []any{"a", "b"} // only 2 values for 3 dests

	err := assignAll(dests, vals)
	if err != nil {
		t.Fatalf("assignAll: %v", err)
	}

	if s1 != "a" || s2 != "b" || s3 != "" {
		t.Errorf("s1=%q s2=%q s3=%q, want a/b/empty", s1, s2, s3)
	}
}

// TestRows_InterfaceMethods verifies the default return values of the
// pgx.Rows interface methods that carry no meaningful data in the fake.
func TestRows_InterfaceMethods(t *testing.T) {
	t.Parallel()

	ctx := t.Context()

	p := &Pool{}
	p.PushRows(nil, []any{"test"})

	rows, err := p.Query(ctx, "SELECT 1")
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	defer rows.Close()

	fakeRows, ok := rows.(*Rows)
	if !ok {
		t.Fatal("expected rows to be *Rows")
	}

	// Test interface methods that return defaults.
	if fakeRows.CommandTag().String() != "" {
		t.Errorf("CommandTag unexpected: %v", fakeRows.CommandTag())
	}

	if fakeRows.FieldDescriptions() != nil {
		t.Error("FieldDescriptions should be nil")
	}

	_, err = fakeRows.Values()
	if err != nil {
		t.Errorf("Values: %v", err)
	}

	if fakeRows.RawValues() != nil {
		t.Error("RawValues should be nil")
	}

	if fakeRows.Conn() != nil {
		t.Error("Conn should be nil")
	}
}

// TestRows_Close verifies that Close on the fake Rows does not panic.
func TestRows_Close(t *testing.T) {
	t.Parallel()

	ctx := t.Context()

	p := &Pool{}
	p.PushRows(nil, []any{"test"})

	rows, err := p.Query(ctx, "SELECT 1")
	if err != nil {
		t.Fatalf("Query: %v", err)
	}

	rows.Close() // should not panic
}

// TestRows_ScanNoCurrentRow verifies that Scan fails when Next has not been
// called yet.
func TestRows_ScanNoCurrentRow(t *testing.T) {
	t.Parallel()

	ctx := t.Context()

	p := &Pool{}
	p.PushRows(nil, []any{"test"})

	rows, _ := p.Query(ctx, "SELECT 1")
	defer rows.Close()

	fakeRows, ok := rows.(*Rows)
	if !ok {
		t.Fatal("expected rows to be *Rows")
	}
	// Scan without calling Next first
	var s string

	err := fakeRows.Scan(&s)
	if err == nil {
		t.Error("expected error when scanning without Next()")
	}
}

// TestAssignOne_FloatConversion verifies that a float64 source is converted
// into a float32 destination.
func TestAssignOne_FloatConversion(t *testing.T) {
	t.Parallel()

	var dst float32

	err := assignOne(&dst, float64(3.14))
	if err != nil {
		t.Fatalf("assignOne float: %v", err)
	}

	if dst < 3.13 || dst > 3.15 {
		t.Errorf("expected ~3.14, got %f", dst)
	}
}

// TestAssignOne_TypeConversion verifies that a convertible source type is
// converted into the destination type.
func TestAssignOne_TypeConversion(t *testing.T) {
	t.Parallel()

	// int32 → int64 via CanConvert
	var dst int64

	err := assignOne(&dst, int32(99))
	if err != nil {
		t.Fatalf("assignOne int32→int64: %v", err)
	}

	if dst != 99 {
		t.Errorf("expected 99, got %d", dst)
	}
}

// TestAssignOne_IncompatibleTypes verifies that an error is returned when the
// source cannot be assigned or converted into the destination type.
func TestAssignOne_IncompatibleTypes(t *testing.T) {
	t.Parallel()

	var dst bool
	// string cannot be assigned to bool and CanConvert returns false
	err := assignOne(&dst, []byte("cannot convert"))
	if err == nil {
		t.Error("expected error for incompatible types")
	}
}

// TestAssignOne_NonPointerDst verifies that an error is returned when the
// destination is not a pointer.
func TestAssignOne_NonPointerDst(t *testing.T) {
	t.Parallel()

	var dst string
	// Pass dst by value (not pointer) — should error
	err := assignOne(dst, "value")
	if err == nil {
		t.Error("expected error when dst is not a pointer")
	}
}

// TestRows_CloseIsIdempotent verifies that calling Close more than once does
// not panic.
func TestRows_CloseIsIdempotent(t *testing.T) {
	t.Parallel()

	ctx := t.Context()

	p := &Pool{}
	p.PushRows(nil, []any{"a"}, []any{"b"})
	rows, _ := p.Query(ctx, "SELECT 1")
	rows.Next()
	rows.Close()
	rows.Close() // calling Close again should not panic
}

// TestRows_Values_WithCurrentRow verifies that Values returns nil even when
// positioned on a current row, matching the fake's documented behaviour.
func TestRows_Values_WithCurrentRow(t *testing.T) {
	t.Parallel()

	ctx := t.Context()

	p := &Pool{}
	p.PushRows(nil, []any{"hello", int64(42)})

	rows, _ := p.Query(ctx, "SELECT 1")
	defer rows.Close()

	fakeRows, ok := rows.(*Rows)
	if !ok {
		t.Fatal("expected rows to be *Rows")
	}

	fakeRows.Next()

	vals, err := fakeRows.Values()
	if err != nil {
		t.Errorf("Values: %v", err)
	}

	if vals != nil {
		t.Errorf("expected nil (Values returns nil always), got %v", vals)
	}
}
