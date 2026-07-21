// specscore: feat-recordops/diff
package recordops_test

import (
	"bytes"
	"encoding/hex"
	"fmt"

	"github.com/dal-go/dalgo/recordops"
	"github.com/dal-go/record"
)

// ExampleDiffFunc shows the canonical DiffFunc use case: comparing two
// recordsets keyed by [16]byte UUIDs, where ID ordering is provided by
// bytes.Compare instead of cmp.Ordered. The baseline has u1 only; the
// candidate has u2 only — one Missing emission and one Extra emission.
func ExampleDiffFunc() {
	type uuid = [16]byte
	u1 := uuid{0x01}
	u2 := uuid{0x02}

	mk := func(id uuid) record.WithID[uuid] {
		key := record.NewKeyWithID("Users", hex.EncodeToString(id[:]))
		r := record.NewRecordWithData(key, map[string]any{"name": "alice"})
		r.SetError(nil)
		return record.WithID[uuid]{ID: id, Record: r}
	}

	// Inputs MUST be sorted ascending by ID.
	baseline := recordops.SliceToSeq([]record.WithID[uuid]{mk(u1)})
	cand := recordops.SliceToSeq([]record.WithID[uuid]{mk(u2)})

	less := func(a, b uuid) bool { return bytes.Compare(a[:], b[:]) < 0 }

	for d, err := range recordops.DiffFunc[uuid](
		baseline,
		[]recordops.RecordSeq[uuid]{cand},
		less,
	) {
		if err != nil {
			fmt.Println("err:", err)
			return
		}
		fmt.Printf("id=%s status=%d\n", hex.EncodeToString(d.ID[:]), d.Candidates[0].Status)
	}

	// Output:
	// id=01000000000000000000000000000000 status=0
	// id=02000000000000000000000000000000 status=1
}
