package access

import (
	"context"
	"fmt"

	"github.com/dal-go/dalgo/condeval"
	"github.com/dal-go/dalgo/dal"
	"github.com/dal-go/record"
	"github.com/dal-go/record/update"
)

// writeResidual is one policy's WriteResidual for one request resource, with
// the attribution a denial needs.
type writeResidual struct {
	policy       string
	policySource string
	resource     Resource
	residual     *WriteResidual
}

func (w writeResidual) deny(operation Operations, rule, condition, explanation string) *DeniedError {
	return &DeniedError{Decision: Decision{
		Operation:    operation,
		Resource:     w.resource,
		Policy:       w.policy,
		PolicySource: w.policySource,
		Rule:         rule,
		Effect:       effectDeny.String(),
		Condition:    condition,
		Explanation:  explanation,
	}}
}

// writeTarget is one record a write touches: its key, the new data for an
// Insert or Set, or the updates for an Update.
type writeTarget struct {
	key     *record.Key
	data    any
	updates []update.Update
}

// writeImages holds what a write residual is evaluated against: whether the
// row exists, its pre-image, and its post-image (nil for a Delete).
type writeImages struct {
	exists bool
	pre    map[string]any
	post   map[string]any
}

// enforceWrites evaluates every policy's write residual for every target
// before any write is delegated, so a batch with one refused row writes
// nothing.
func (s securedWriteSession) enforceWrites(ctx context.Context, operation Operations, writes [][]writeResidual, targets []writeTarget) error {
	if len(writes) == 0 {
		return nil
	}
	for i, target := range targets {
		perResource := writes[i]
		if len(perResource) == 0 {
			continue
		}
		images, err := s.writeImages(ctx, operation, target, perResource[0])
		if err != nil {
			return err
		}
		for _, w := range perResource {
			if err := evaluateWrite(operation, images, w); err != nil {
				return err
			}
		}
	}
	return nil
}

// writeImages reads the pre-image (inside the caller's transaction) and
// computes the post-image for one target.
func (s securedWriteSession) writeImages(ctx context.Context, operation Operations, target writeTarget, w writeResidual) (writeImages, error) {
	var images writeImages
	if operation != Insert {
		reader, ok := s.session.(dal.ReadSession)
		if !ok {
			return images, w.deny(operation, "", "", "a conditional write needs a session that can read the row's pre-image")
		}
		shadow := record.NewRecordWithData(target.key, &map[string]any{})
		if err := reader.Get(ctx, shadow); err != nil {
			return images, err
		}
		if shadow.Exists() {
			images.exists = true
			images.pre = *shadow.Data().(*map[string]any)
		}
	}
	switch operation {
	case Insert, Set:
		post, err := condeval.ToMap(target.data)
		if err != nil {
			return images, w.deny(operation, "", "", fmt.Sprintf("row data could not be evaluated: %v", err))
		}
		images.post = post
	case Update:
		if images.exists {
			post := condeval.CloneMap(images.pre)
			if err := condeval.ApplyUpdates(post, target.updates); err != nil {
				return images, w.deny(operation, "", "", fmt.Sprintf("post-image could not be computed: %v", err))
			}
			images.post = post
		}
	}
	return images, nil
}

// evaluateWrite applies one policy's write residual. The first alternative
// whose Where holds on the pre-image decides an Update, Set of an existing row
// or Delete; a new row is admitted by the first alternative whose Check it
// satisfies; the Terminal allow applies when no alternative does.
func evaluateWrite(operation Operations, images writeImages, w writeResidual) error {
	r := w.residual
	isNewRow := operation == Insert || (operation == Set && !images.exists)
	if isNewRow {
		for _, alternative := range r.Alternatives {
			check, checkText := alternativeCheck(alternative)
			ok, err := condeval.Match(images.post, check)
			if err != nil {
				return w.deny(operation, alternative.Rule, checkText, fmt.Sprintf("post-image check could not be evaluated for rule %q: %v", alternative.Rule, err))
			}
			if ok {
				return nil
			}
		}
		return terminalAdmits(operation, images, w, "no rule admits the new row")
	}
	if !images.exists {
		// Nothing to protect: the adapter reports the missing row unless an
		// unconditional allow applies.
		if r.Terminal != nil {
			return nil
		}
		return w.deny(operation, "", "", "row does not exist or is outside every conditional rule")
	}
	for _, alternative := range r.Alternatives {
		ok, err := condeval.Match(images.pre, alternative.Where)
		if err != nil {
			return w.deny(operation, alternative.Rule, alternative.WhereText, fmt.Sprintf("row condition could not be evaluated for rule %q: %v", alternative.Rule, err))
		}
		if !ok {
			continue
		}
		if operation == Delete {
			return nil
		}
		check, checkText := alternativeCheck(alternative)
		ok, err = condeval.Match(images.post, check)
		if err != nil {
			return w.deny(operation, alternative.Rule, checkText, fmt.Sprintf("post-image check could not be evaluated for rule %q: %v", alternative.Rule, err))
		}
		if !ok {
			return w.deny(operation, alternative.Rule, checkText, fmt.Sprintf("post-image check not satisfied for rule %q (check: %s)", alternative.Rule, checkText))
		}
		return nil
	}
	return terminalAdmits(operation, images, w, "row condition not satisfied for any rule")
}

// alternativeCheck returns the post-image condition of an alternative: its
// Check, or its Where when no Check is declared.
func alternativeCheck(alternative WriteAlternative) (dal.Condition, string) {
	if alternative.Check != nil {
		return alternative.Check, alternative.CheckText
	}
	return alternative.Where, alternative.WhereText
}

// terminalAdmits applies the Terminal allow, if any, to a write no
// alternative decided: a Delete needs no check; other writes must satisfy the
// terminal's Check when it has one.
func terminalAdmits(operation Operations, images writeImages, w writeResidual, failure string) error {
	terminal := w.residual.Terminal
	if terminal == nil {
		return w.deny(operation, alternativeNames(w.residual), alternativeTexts(w.residual), failure)
	}
	if operation == Delete || terminal.Check == nil {
		return nil
	}
	ok, err := condeval.Match(images.post, terminal.Check)
	if err != nil {
		return w.deny(operation, terminal.Rule, terminal.CheckText, fmt.Sprintf("post-image check could not be evaluated for rule %q: %v", terminal.Rule, err))
	}
	if !ok {
		return w.deny(operation, terminal.Rule, terminal.CheckText, fmt.Sprintf("post-image check not satisfied for rule %q (check: %s)", terminal.Rule, terminal.CheckText))
	}
	return nil
}

func alternativeNames(r *WriteResidual) string {
	names := make([]string, len(r.Alternatives))
	for i, alternative := range r.Alternatives {
		names[i] = alternative.Rule
	}
	return joinNames(names)
}

func alternativeTexts(r *WriteResidual) string {
	texts := make([]string, len(r.Alternatives))
	for i, alternative := range r.Alternatives {
		texts[i] = alternative.WhereText
	}
	if len(texts) == 1 {
		return texts[0]
	}
	return "(" + joinWith(texts, " OR ") + ")"
}

func joinNames(names []string) string { return joinWith(names, ", ") }

func joinWith(parts []string, separator string) string {
	out := ""
	for i, part := range parts {
		if i > 0 {
			out += separator
		}
		out += part
	}
	return out
}
