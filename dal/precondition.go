package dal

import "time"

// Precondition defines precondition
type Precondition = func(preconditions *preConditions)

// Preconditions defines preconditions
type Preconditions interface {
	Exists() bool
}

type preConditions struct {
	exists         bool
	lastUpdateTime time.Time
}

// Exists indicate exists precondition
func (v preConditions) Exists() bool {
	return v.exists
}

// WithExistsPrecondition sets exists precondition
func WithExistsPrecondition() func(preconditions *preConditions) {
	return func(preconditions *preConditions) {
		preconditions.exists = true
	}
}

// WithLastUpdateTimePrecondition sets last update time
func WithLastUpdateTimePrecondition(t time.Time) func(preconditions *preConditions) {
	return func(preconditions *preConditions) {
		preconditions.lastUpdateTime = t
	}
}

// GetPreconditions create Preconditions
func GetPreconditions(items ...Precondition) Preconditions {
	var result preConditions
	for _, precondition := range items {
		precondition(&result)
	}
	return result
}
