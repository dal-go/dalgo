// specscore: feat-recordops/diff
package recordops

import (
	"cmp"
	"fmt"
	"iter"

	"github.com/dal-go/record"
)

// Diff compares baseline against candidates via K-way merge over
// ID-sorted streams and yields one IDDiff per ID where at least one
// candidate diverges (default) or every ID touched by any input
// (with WithIncludeMatched).
//
// Inputs MUST be sorted ascending by ID. Monotonicity is validated
// per stream; violations terminate with ErrUnsortedInput. Duplicate
// IDs within a stream terminate with ErrDuplicateID. Upstream stream
// errors propagate verbatim.
//
// Diff requires K to be cmp.Ordered (string/int/float/etc.). For
// types that are comparable but not orderable (e.g., [16]byte UUIDs),
// use DiffFunc with an explicit less function.
//
// See Package recordops doc for the K-way merge model and memory
// footprint. See spec/features/recordops/diff for the full contract.
//
// Renderers consume the returned stream once; multi-view consumers
// must materialize first (slices.Collect-equivalent).
func Diff[K cmp.Ordered](
	baseline RecordSeq[K],
	candidates []RecordSeq[K],
	opts ...Option,
) iter.Seq2[IDDiff[K], error] {
	return DiffFunc[K](baseline, candidates, func(a, b K) bool { return a < b }, opts...)
}

// DiffFunc is Diff for any K comparable, with caller-supplied strict
// weak order. For UUID-keyed records typed as [16]byte, pass
// bytes.Compare(a[:], b[:]) < 0 as less.
//
// less MUST be a strict weak order (irreflexive, antisymmetric,
// transitive). If less is nil, the returned stream yields exactly
// one (zero, ErrInvalidArgument) and stops.
func DiffFunc[K comparable](
	baseline RecordSeq[K],
	candidates []RecordSeq[K],
	less func(a, b K) bool,
	opts ...Option,
) iter.Seq2[IDDiff[K], error] {
	cfg := resolveOptions(opts...)
	return func(yield func(IDDiff[K], error) bool) {
		var zero IDDiff[K]
		if less == nil {
			yield(zero, fmt.Errorf("recordops.DiffFunc: less must not be nil: %w", ErrInvalidArgument))
			return
		}

		// One cursor per input stream: index 0 = baseline, indexes 1..N = candidates.
		baseCur := newCursor(baseline, "baseline")
		candCurs := make([]*streamCursor[K], len(candidates))
		for i, c := range candidates {
			candCurs[i] = newCursor(c, fmt.Sprintf("candidates[%d]", i))
		}

		// Defer stop on all cursors so iter.Seq2 source can clean up.
		defer baseCur.stop()
		defer func() {
			for _, c := range candCurs {
				c.stop()
			}
		}()

		// Prime: pull one record from each stream.
		if err := baseCur.advance(less); err != nil {
			yield(zero, err)
			return
		}
		for _, c := range candCurs {
			if err := c.advance(less); err != nil {
				yield(zero, err)
				return
			}
		}

		// Merge loop: repeatedly find smallest live ID across all cursors.
		for {
			id, anyLive := smallestID(baseCur, candCurs, less)
			if !anyLive {
				return
			}

			diff, err := assembleIDDiff(id, baseCur, candCurs, cfg)
			if err != nil {
				yield(zero, err)
				return
			}

			// Advance every cursor that was at this id.
			if baseCur.live && baseCur.id == id {
				if err := baseCur.advance(less); err != nil {
					yield(zero, err)
					return
				}
			}
			for _, c := range candCurs {
				if c.live && c.id == id {
					if err := c.advance(less); err != nil {
						yield(zero, err)
						return
					}
				}
			}

			// Emit iff non-Matched OR WithIncludeMatched.
			if shouldEmit(diff, cfg) {
				if !yield(diff, nil) {
					return
				}
			}
		}
	}
}

// streamCursor wraps an iter.Seq2 with explicit "current id" and
// monotonicity-checking advance. Uses iter.Pull2 for one-step access.
type streamCursor[K comparable] struct {
	name    string
	next    func() (record.WithID[K], error, bool)
	stop    func()
	live    bool
	id      K
	rec     record.WithID[K]
	prev    K
	hasPrev bool
}

func newCursor[K comparable](s RecordSeq[K], name string) *streamCursor[K] {
	next, stop := iter.Pull2(s)
	return &streamCursor[K]{name: name, next: next, stop: stop, live: true}
}

func (c *streamCursor[K]) advance(less func(a, b K) bool) error {
	r, err, ok := c.next()
	if !ok {
		c.live = false
		return nil
	}
	if err != nil {
		c.live = false
		return err
	}
	if c.hasPrev {
		// Strict ascending: less(prev, r.ID) must hold. If not, either
		// equal (duplicate) or reversed (unsorted).
		if !less(c.prev, r.ID) {
			if c.prev == r.ID {
				return fmt.Errorf("recordops: duplicate ID %v in %s: %w", r.ID, c.name, ErrDuplicateID)
			}
			return fmt.Errorf("recordops: stream out of order at id %v in %s: %w", r.ID, c.name, ErrUnsortedInput)
		}
	}
	c.prev = r.ID
	c.hasPrev = true
	c.id = r.ID
	c.rec = r
	return nil
}

func smallestID[K comparable](
	base *streamCursor[K],
	cands []*streamCursor[K],
	less func(a, b K) bool,
) (K, bool) {
	var smallest K
	any := false
	consider := func(c *streamCursor[K]) {
		if !c.live {
			return
		}
		if !any || less(c.id, smallest) {
			smallest = c.id
			any = true
		}
	}
	consider(base)
	for _, c := range cands {
		consider(c)
	}
	return smallest, any
}

func assembleIDDiff[K comparable](
	id K,
	base *streamCursor[K],
	cands []*streamCursor[K],
	cfg options,
) (IDDiff[K], error) {
	out := IDDiff[K]{ID: id, Candidates: make([]CandidateState, len(cands))}

	var baseRec *record.WithID[K]
	if base.live && base.id == id {
		baseRec = &base.rec
	}

	for i, c := range cands {
		var candRec *record.WithID[K]
		if c.live && c.id == id {
			candRec = &c.rec
		}
		state, err := classify(baseRec, candRec, cfg)
		if err != nil {
			return IDDiff[K]{}, err
		}
		out.Candidates[i] = state
	}

	// Baseline snapshot. Full population by default; trimmed under WithOnlyChangedFields.
	if baseRec != nil {
		out.Baseline = buildBaselineSnapshot(*baseRec, out.Candidates, cfg)
	}

	return out, nil
}

// classify returns the CandidateState for one (baseline, candidate) pair.
// "Both present" delegates to compareRecords for field-level deltas.
func classify[K comparable](base, cand *record.WithID[K], cfg options) (CandidateState, error) {
	switch {
	case cand == nil:
		// candidate lacks this ID (whether baseline has it or another
		// candidate surfaced it) → Missing from this candidate's view.
		return CandidateState{Status: Missing}, nil
	case base == nil:
		// candidate-only → Extra. Emit candidate's full field list.
		return CandidateState{Status: Extra, Fields: extractAllFields(cand.Record)}, nil
	default:
		deltas, err := compareRecords(base.ID, base.Record, cand.Record, cfg)
		if err != nil {
			return CandidateState{}, err
		}
		if len(deltas) == 0 {
			return CandidateState{Status: Matched}, nil
		}
		return CandidateState{Status: Changed, Fields: deltas}, nil
	}
}

func buildBaselineSnapshot[K comparable](base record.WithID[K], cs []CandidateState, cfg options) *RecordSnapshot {
	perCandidateDeltas := make([][]FieldValue, len(cs))
	for i, c := range cs {
		perCandidateDeltas[i] = c.Fields
	}
	return &RecordSnapshot{Fields: baselineFields(base.Record, perCandidateDeltas, cfg)}
}

func shouldEmit[K comparable](d IDDiff[K], cfg options) bool {
	if cfg.includeMatched {
		return true
	}
	for _, c := range d.Candidates {
		if c.Status != Matched {
			return true
		}
	}
	return false
}
