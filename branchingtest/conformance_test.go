package branchingtest

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/dal-go/dalgo/adapters/dalgo2memory"
	"github.com/dal-go/dalgo/branching"
	"github.com/dal-go/dalgo/dal"
	"github.com/dal-go/record"
)

type conformanceItem struct {
	Count int `json:"count"`
}

var conformanceKey = record.NewKeyWithID("items", "one")

type conformanceProvider struct{}

func (conformanceProvider) Capability() branching.Capability {
	return branching.Capability{Provider: "test", Version: "1", Mode: "serialized"}
}

func (conformanceProvider) Capture(ctx context.Context, source dal.DB) (branching.Checkpoint, error) {
	item, exists, err := readConformanceItem(ctx, source)
	if err != nil {
		return nil, err
	}
	return &conformanceCheckpoint{item: item, exists: exists}, nil
}

type conformanceCheckpoint struct {
	item     conformanceItem
	exists   bool
	released bool
}

func (*conformanceCheckpoint) Generation() string { return "test-generation" }

func (c *conformanceCheckpoint) Branch(ctx context.Context) (branching.Branch, error) {
	if c.released {
		return nil, branching.ErrReleased
	}
	db := dalgo2memory.NewDB()
	if c.exists {
		if err := setConformanceItem(ctx, db, c.item.Count); err != nil {
			return nil, err
		}
	}
	return &conformanceBranch{db: db}, nil
}

func (c *conformanceCheckpoint) Release(context.Context) error {
	c.released = true
	return nil
}

type conformanceBranch struct {
	db     dal.DB
	closed bool
}

func (b *conformanceBranch) DB() dal.DB { return b.db }

func (b *conformanceBranch) Close(context.Context) error {
	b.closed = true
	return nil
}

func TestRunConformance(t *testing.T) {
	RunConformance(t, Callbacks{
		New: func(testing.TB) (dal.DB, branching.Provider) {
			return dalgo2memory.NewDB(), conformanceProvider{}
		},
		Seed: func(ctx context.Context, db dal.DB) error {
			return setConformanceItem(ctx, db, 1)
		},
		Mutate: func(ctx context.Context, db dal.DB) error {
			return setConformanceItem(ctx, db, 2)
		},
		Digest: func(ctx context.Context, db dal.DB) (string, error) {
			item, exists, err := readConformanceItem(ctx, db)
			if err != nil {
				return "", err
			}
			if !exists {
				return "empty", nil
			}
			return fmt.Sprintf("count=%d", item.Count), nil
		},
	})
}

func TestFailurePathCoverage(t *testing.T) {
	tests := map[string]struct {
		subtest  string
		expected string
	}{
		"validate":                  {expected: "New, Seed, Mutate and Digest callbacks are required"},
		"capture nil input":         {expected: "factory returned nil source or provider"},
		"capture error":             {expected: "Capture(): capture failed"},
		"capture nil checkpoint":    {expected: "Capture() returned nil checkpoint"},
		"capture empty generation":  {expected: "checkpoint generation is empty"},
		"branch error":              {expected: "Branch(): branch failed"},
		"branch nil":                {expected: "Branch() returned nil branch or database"},
		"branch source handle":      {expected: "Branch() returned the live source dal.DB handle"},
		"seed error":                {expected: "Seed(): seed failed"},
		"mutate error":              {expected: "Mutate(): mutate failed"},
		"digest error":              {expected: "Digest(): digest failed"},
		"close error":               {expected: "Close(): close failed"},
		"release error":             {expected: "Release(): release failed"},
		"empty digest mismatch":     {subtest: "empty_database", expected: "empty branch digest"},
		"seed digest mismatch":      {subtest: "seed_fidelity_and_fresh_handle", expected: "seeded branch digest"},
		"source mutation unchanged": {subtest: "source_mutation_does_not_change_checkpoint", expected: "mutation did not change source digest"},
		"checkpoint follows source": {subtest: "source_mutation_does_not_change_checkpoint", expected: "branch after source mutation digest"},
		"first sibling unchanged":   {subtest: "sibling_isolation", expected: "first sibling mutation did not change digest"},
		"siblings same handle":      {subtest: "sibling_isolation", expected: "siblings received the same dal.DB handle"},
		"second sibling mismatch":   {subtest: "sibling_isolation", expected: "second sibling digest"},
		"third sibling mismatch":    {subtest: "sibling_isolation", expected: "checkpoint changed after sibling mutation"},
		"first release error":       {subtest: "release_is_idempotent", expected: "first Release(): release failed"},
		"second release error":      {subtest: "release_is_idempotent", expected: "second Release(): release failed"},
		"branch after release":      {subtest: "release_is_idempotent", expected: "want ErrReleased"},
	}
	for failureCase, test := range tests {
		t.Run(failureCase, func(t *testing.T) {
			runPattern := "^TestConformanceFailureHelper$"
			if test.subtest != "" {
				runPattern += "/^" + test.subtest + "$"
			}
			cmd := exec.Command(os.Args[0], append([]string{
				"-test.run=" + runPattern,
				"-test.count=1",
			}, coverageArguments()...)...)
			cmd.Env = append(os.Environ(), "DALGO_BRANCHINGTEST_FAILURE="+failureCase)
			output, err := cmd.CombinedOutput()
			if err == nil {
				t.Fatalf("failure helper unexpectedly passed:\n%s", output)
			}
			if !strings.Contains(string(output), test.expected) {
				t.Fatalf("failure helper output did not contain %q:\n%s", test.expected, output)
			}
		})
	}
}

func TestConformanceFailureHelper(t *testing.T) {
	failureCase := os.Getenv("DALGO_BRANCHINGTEST_FAILURE")
	if failureCase == "" {
		t.Skip("only run as a subprocess by TestFailurePathCoverage")
	}
	source := dalgo2memory.NewDB()
	switch failureCase {
	case "validate":
		validate(t, Callbacks{})
	case "capture nil input":
		capture(t, nil, nil)
	case "capture error":
		capture(t, failureProvider{captureErr: errors.New("capture failed")}, source)
	case "capture nil checkpoint":
		capture(t, failureProvider{}, source)
	case "capture empty generation":
		capture(t, failureProvider{checkpoint: &failureCheckpoint{}}, source)
	case "branch error":
		startBranch(t, &failureCheckpoint{generation: "generation", branchErr: errors.New("branch failed")}, source)
	case "branch nil":
		startBranch(t, &failureCheckpoint{generation: "generation"}, source)
	case "branch source handle":
		startBranch(t, &failureCheckpoint{
			generation: "generation",
			branch:     &failureBranch{db: source},
		}, source)
	case "seed error":
		seed(t, Callbacks{Seed: func(context.Context, dal.DB) error {
			return errors.New("seed failed")
		}}, source)
	case "mutate error":
		mutate(t, Callbacks{Mutate: func(context.Context, dal.DB) error {
			return errors.New("mutate failed")
		}}, source)
	case "digest error":
		digest(t, Callbacks{Digest: func(context.Context, dal.DB) (string, error) {
			return "", errors.New("digest failed")
		}}, source)
	case "close error":
		closeBranch(t, &failureBranch{closeErr: errors.New("close failed")})
	case "release error":
		release(t, &failureCheckpoint{releaseErr: errors.New("release failed")})
	case "empty digest mismatch", "seed digest mismatch", "source mutation unchanged",
		"checkpoint follows source", "first sibling unchanged", "siblings same handle",
		"second sibling mismatch", "third sibling mismatch", "first release error",
		"second release error", "branch after release":
		RunConformance(t, scriptedFailureCallbacks(failureCase))
	default:
		t.Fatalf("unknown failure case %q", failureCase)
	}
}

func coverageArguments() []string {
	var args []string
	for _, arg := range os.Args[1:] {
		if strings.HasPrefix(arg, "-test.gocoverdir=") {
			args = append(args, arg)
		}
	}
	return args
}

type failureProvider struct {
	checkpoint branching.Checkpoint
	captureErr error
}

func (failureProvider) Capability() branching.Capability { return branching.Capability{} }

func (p failureProvider) Capture(context.Context, dal.DB) (branching.Checkpoint, error) {
	return p.checkpoint, p.captureErr
}

type failureCheckpoint struct {
	generation string
	branch     branching.Branch
	branchErr  error
	releaseErr error
}

func (c *failureCheckpoint) Generation() string { return c.generation }

func (c *failureCheckpoint) Branch(context.Context) (branching.Branch, error) {
	return c.branch, c.branchErr
}

func (c *failureCheckpoint) Release(context.Context) error { return c.releaseErr }

type failureBranch struct {
	db       dal.DB
	closeErr error
}

func (b *failureBranch) DB() dal.DB { return b.db }

func (b *failureBranch) Close(context.Context) error { return b.closeErr }

type conformanceFailureScript struct {
	failureCase string
	source      dal.DB
	states      map[dal.DB]string
}

func scriptedFailureCallbacks(failureCase string) Callbacks {
	script := &conformanceFailureScript{
		failureCase: failureCase,
		states:      make(map[dal.DB]string),
	}
	return Callbacks{
		New: func(testing.TB) (dal.DB, branching.Provider) {
			script.source = dalgo2memory.NewDB()
			return script.source, &scriptedProvider{script: script}
		},
		Seed: func(_ context.Context, db dal.DB) error {
			script.states[db] = "baseline"
			return nil
		},
		Mutate: func(_ context.Context, db dal.DB) error {
			if failureCase == "source mutation unchanged" ||
				(failureCase == "first sibling unchanged" && db != script.source) {
				return nil
			}
			script.states[db] = "mutated"
			return nil
		},
		Digest: func(_ context.Context, db dal.DB) (string, error) {
			if state, ok := script.states[db]; ok {
				return state, nil
			}
			return "empty", nil
		},
	}
}

type scriptedProvider struct {
	script *conformanceFailureScript
}

func (*scriptedProvider) Capability() branching.Capability { return branching.Capability{} }

func (p *scriptedProvider) Capture(context.Context, dal.DB) (branching.Checkpoint, error) {
	state := "empty"
	if captured, ok := p.script.states[p.script.source]; ok {
		state = captured
	}
	return &scriptedCheckpoint{script: p.script, captured: state}, nil
}

type scriptedCheckpoint struct {
	script       *conformanceFailureScript
	captured     string
	branchCalls  int
	releaseCalls int
	released     bool
	sharedDB     dal.DB
}

func (*scriptedCheckpoint) Generation() string { return "scripted-generation" }

func (c *scriptedCheckpoint) Branch(context.Context) (branching.Branch, error) {
	if c.released {
		if c.script.failureCase == "branch after release" {
			return nil, nil
		}
		return nil, branching.ErrReleased
	}
	c.branchCalls++
	db := dalgo2memory.NewDB()
	if c.script.failureCase == "siblings same handle" {
		if c.sharedDB == nil {
			c.sharedDB = db
		}
		db = c.sharedDB
	}
	state := c.captured
	switch {
	case c.script.failureCase == "empty digest mismatch":
		state = "different"
	case c.script.failureCase == "seed digest mismatch":
		state = "different"
	case c.script.failureCase == "checkpoint follows source":
		state = c.script.states[c.script.source]
	case c.script.failureCase == "second sibling mismatch" && c.branchCalls == 2:
		state = "different"
	case c.script.failureCase == "third sibling mismatch" && c.branchCalls == 3:
		state = "different"
	}
	if _, exists := c.script.states[db]; !exists || c.script.failureCase != "siblings same handle" {
		c.script.states[db] = state
	}
	return &failureBranch{db: db}, nil
}

func (c *scriptedCheckpoint) Release(context.Context) error {
	c.releaseCalls++
	c.released = true
	if (c.script.failureCase == "first release error" && c.releaseCalls == 1) ||
		(c.script.failureCase == "second release error" && c.releaseCalls == 2) {
		return errors.New("release failed")
	}
	return nil
}

func setConformanceItem(ctx context.Context, db dal.DB, count int) error {
	writer, ok := db.(interface {
		Set(context.Context, record.Record) error
	})
	if !ok {
		return errors.New("database cannot set records")
	}
	return writer.Set(ctx, record.NewRecordWithData(conformanceKey, &conformanceItem{Count: count}))
}

func readConformanceItem(ctx context.Context, db dal.DB) (item conformanceItem, exists bool, err error) {
	err = db.Get(ctx, record.NewRecordWithData(conformanceKey, &item))
	if record.IsNotFound(err) {
		return conformanceItem{}, false, nil
	}
	return item, err == nil, err
}
