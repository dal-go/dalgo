package recordset

import (
	"errors"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

// fakeEvaluator records the keys it was given and the number of invocations.
type fakeEvaluator struct {
	eval     func(stored map[string]any) (any, error)
	calls    int
	lastKeys []string
}

func (e *fakeEvaluator) Eval(stored map[string]any) (any, error) {
	e.calls++
	e.lastKeys = make([]string, 0, len(stored))
	for k := range stored {
		e.lastKeys = append(e.lastKeys, k)
	}
	return e.eval(stored)
}

// evaluator-compiles: a user-defined type with method Eval(map[string]any)(any,error)
// is assignable to recordset.Evaluator.
func TestEvaluatorCompiles(t *testing.T) {
	var e Evaluator = &fakeEvaluator{eval: func(map[string]any) (any, error) { return nil, nil }}
	assert.NotNil(t, e)
}

// recordset-has-no-scripting-dependency: the package imports no formula/scripting runtime.
func TestNoScriptingDependency(t *testing.T) {
	forbidden := []string{"scripting", "formula", "expr", "govaluate", "starlark", "lua", "otto", "goja"}
	fset := token.NewFileSet()
	entries, err := os.ReadDir(".")
	assert.NoError(t, err)
	for _, entry := range entries {
		name := entry.Name()
		if !strings.HasSuffix(name, ".go") {
			continue
		}
		f, err := parser.ParseFile(fset, filepath.Join(".", name), nil, parser.ImportsOnly)
		assert.NoError(t, err)
		for _, imp := range f.Imports {
			path := strings.Trim(imp.Path.Value, `"`)
			for _, bad := range forbidden {
				assert.NotContains(t, path, bad, "file %s imports forbidden package %s", name, path)
			}
		}
	}
}

func TestComputedColumn(t *testing.T) {
	concat := &fakeEvaluator{eval: func(stored map[string]any) (any, error) {
		return stored["first"].(string) + " " + stored["last"].(string), nil
	}}

	newRS := func() *ColumnarRecordset {
		return NewColumnarRecordset("people",
			NewColumn[string]("first", ""),
			NewColumn[string]("last", ""),
			NewComputedColumn("full", concat),
		)
	}

	// computed-column-registers-and-marks
	t.Run("registers-and-marks", func(t *testing.T) {
		e := &fakeEvaluator{eval: func(map[string]any) (any, error) { return nil, nil }}
		rs := NewColumnarRecordset("rs",
			NewColumn[int]("qty", 0),
			NewComputedColumn("label", e),
		)
		cc, ok := rs.GetColumnByName("label").(ComputedColumn)
		assert.True(t, ok)
		assert.Same(t, e, cc.Evaluator().(*fakeEvaluator))

		_, ok = rs.GetColumnByName("qty").(ComputedColumn)
		assert.False(t, ok)
	})

	// computedColumn standalone behaviour (Name, fail-loud GetValue, SetValue reject, helpers).
	t.Run("standalone-column-behaviour", func(t *testing.T) {
		e := &fakeEvaluator{eval: func(map[string]any) (any, error) { return nil, nil }}
		col := NewComputedColumn("c", e, ColDbType("computed"))
		assert.Equal(t, "c", col.Name())
		assert.Equal(t, "computed", col.DbType())
		assert.Nil(t, col.DefaultValue())
		assert.False(t, col.IsBitmap())
		assert.NotNil(t, col.ValueType())
		assert.Nil(t, col.Values())
		assert.NoError(t, col.Add(1))

		_, err := col.GetValue(0)
		assert.Error(t, err)

		err = col.SetValue(0, "x")
		assert.Error(t, err)
	})

	// lazy-resolves-from-siblings
	t.Run("lazy-resolves-from-siblings", func(t *testing.T) {
		rs := newRS()
		row := rs.NewRow()
		assert.NoError(t, row.SetValueByName("first", "Ada", rs))
		assert.NoError(t, row.SetValueByName("last", "Lovelace", rs))

		row0 := rs.GetRow(0)
		val, err := row0.GetValueByName("full", rs)
		assert.NoError(t, err)
		assert.Equal(t, "Ada Lovelace", val)
	})

	// input-excludes-computed
	t.Run("input-excludes-computed", func(t *testing.T) {
		recorded := &fakeEvaluator{eval: func(stored map[string]any) (any, error) {
			return stored["first"].(string) + " " + stored["last"].(string), nil
		}}
		greet := &fakeEvaluator{eval: func(map[string]any) (any, error) { return "hi", nil }}
		rs := NewColumnarRecordset("people",
			NewColumn[string]("first", ""),
			NewColumn[string]("last", ""),
			NewComputedColumn("full", recorded),
			NewComputedColumn("greeting", greet),
		)
		row := rs.NewRow()
		assert.NoError(t, row.SetValueByName("first", "Ada", rs))
		assert.NoError(t, row.SetValueByName("last", "Lovelace", rs))

		_, err := row.GetValueByName("full", rs)
		assert.NoError(t, err)
		assert.ElementsMatch(t, []string{"first", "last"}, recorded.lastKeys)
	})

	// returns-raw-uncoerced-value
	t.Run("returns-raw-uncoerced-value", func(t *testing.T) {
		intEval := &fakeEvaluator{eval: func(map[string]any) (any, error) { return 42, nil }}
		rs := NewColumnarRecordset("rs",
			NewColumn[string]("first", ""),
			NewComputedColumn("answer", intEval),
		)
		row := rs.NewRow()
		val, err := row.GetValueByName("answer", rs)
		assert.NoError(t, err)
		assert.Equal(t, 42, val)
	})

	// memoized-single-eval
	t.Run("memoized-single-eval", func(t *testing.T) {
		counter := &fakeEvaluator{eval: func(map[string]any) (any, error) { return "fixed", nil }}
		rs := NewColumnarRecordset("rs",
			NewColumn[string]("first", ""),
			NewComputedColumn("c", counter),
		)
		_ = rs.NewRow()
		row := rs.GetRow(0)
		v1, err1 := row.GetValueByName("c", rs)
		v2, err2 := row.GetValueByName("c", rs)
		v3, err3 := row.GetValueByName("c", rs)
		assert.NoError(t, err1)
		assert.NoError(t, err2)
		assert.NoError(t, err3)
		assert.Equal(t, 1, counter.calls)
		assert.Equal(t, "fixed", v1)
		assert.Equal(t, "fixed", v2)
		assert.Equal(t, "fixed", v3)
	})

	// eval-error-propagates-and-caches
	t.Run("eval-error-propagates-and-caches", func(t *testing.T) {
		boom := errors.New("boom")
		failing := &fakeEvaluator{eval: func(map[string]any) (any, error) { return nil, boom }}
		rs := NewColumnarRecordset("rs",
			NewColumn[string]("first", ""),
			NewComputedColumn("c", failing),
		)
		_ = rs.NewRow()
		row := rs.GetRow(0)
		v1, err1 := row.GetValueByName("c", rs)
		v2, err2 := row.GetValueByName("c", rs)
		assert.ErrorIs(t, err1, boom)
		assert.ErrorIs(t, err2, boom)
		assert.Nil(t, v1)
		assert.Nil(t, v2)
		assert.Equal(t, 1, failing.calls)
	})

	// stored-sibling read error propagates and caches (covers error branch in resolveComputed).
	t.Run("stored-sibling-error-propagates", func(t *testing.T) {
		e := &fakeEvaluator{eval: func(map[string]any) (any, error) { return "x", nil }}
		rs := NewColumnarRecordset("rs",
			NewColumn[string]("first", ""),
			NewComputedColumn("c", e),
		)
		// No NewRow() called, so the stored column has no value at row 0 -> GetValue errors.
		row := &columnarRow{i: 0}
		v, err := row.GetValueByName("c", rs)
		assert.Error(t, err)
		assert.Nil(t, v)
		assert.Equal(t, 0, e.calls)
		// second access uses cache, still no eval call
		_, err2 := row.GetValueByName("c", rs)
		assert.Error(t, err2)
		assert.Equal(t, 0, e.calls)
	})

	// data-materializes-computed
	t.Run("data-materializes-computed", func(t *testing.T) {
		rs := newRS()
		row := rs.NewRow()
		assert.NoError(t, row.SetValueByName("first", "Ada", rs))
		assert.NoError(t, row.SetValueByName("last", "Lovelace", rs))

		data, err := rs.GetRow(0).Data(rs)
		assert.NoError(t, err)
		fullIdx := rs.GetColumnIndex("full")
		assert.Equal(t, "Ada Lovelace", data[fullIdx])
	})

	// data returns error when a computed Eval errors.
	t.Run("data-propagates-computed-error", func(t *testing.T) {
		boom := errors.New("boom")
		failing := &fakeEvaluator{eval: func(map[string]any) (any, error) { return nil, boom }}
		rs := NewColumnarRecordset("rs",
			NewColumn[string]("first", ""),
			NewComputedColumn("c", failing),
		)
		row := rs.NewRow()
		assert.NoError(t, row.SetValueByName("first", "Ada", rs))
		_, err := rs.GetRow(0).Data(rs)
		assert.ErrorIs(t, err, boom)
	})

	// reject-set-on-computed (by name and by index)
	t.Run("reject-set-on-computed", func(t *testing.T) {
		rs := newRS()
		row := rs.NewRow()
		assert.NoError(t, row.SetValueByName("first", "Ada", rs))
		assert.NoError(t, row.SetValueByName("last", "Lovelace", rs))

		err := row.SetValueByName("full", "x", rs)
		assert.Error(t, err)

		val, err := row.GetValueByName("full", rs)
		assert.NoError(t, err)
		assert.Equal(t, "Ada Lovelace", val)

		fullIdx := rs.GetColumnIndex("full")
		err = rs.GetRow(0).SetValueByIndex(fullIdx, "y", rs)
		assert.Error(t, err)
	})

	// computed column resolved by index.
	t.Run("get-by-index", func(t *testing.T) {
		rs := newRS()
		row := rs.NewRow()
		assert.NoError(t, row.SetValueByName("first", "Ada", rs))
		assert.NoError(t, row.SetValueByName("last", "Lovelace", rs))

		fullIdx := rs.GetColumnIndex("full")
		val, err := rs.GetRow(0).GetValueByIndex(fullIdx, rs)
		assert.NoError(t, err)
		assert.Equal(t, "Ada Lovelace", val)
	})
}
