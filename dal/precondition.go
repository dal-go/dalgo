package dal

import "time"

type precondition struct {
	f func(preconditions *preConditions)
}

func (v precondition) apply(preconditions *preConditions) {
	v.f(preconditions)
}

// Precondition defines precondition
type Precondition interface {
	apply(preconditions *preConditions)
}

// Preconditions defines preconditions
type Preconditions interface {
	Exists() bool
	LastUpdateTime() time.Time
}

type preConditions struct {
	exists         bool
	lastUpdateTime time.Time
}

// Exists indicate exists precondition
func (v preConditions) Exists() bool {
	return v.exists
}

// LastUpdateTime indicate last update time precondition
func (v preConditions) LastUpdateTime() time.Time {
	return v.lastUpdateTime
}

// WithExistsPrecondition sets exists precondition
func WithExistsPrecondition() Precondition {
	return precondition{f: func(preconditions *preConditions) {
		preconditions.exists = true
	}}
}

// WithLastUpdateTimePrecondition sets last update time
func WithLastUpdateTimePrecondition(t time.Time) Precondition {
	return precondition{f: func(preconditions *preConditions) {
		preconditions.lastUpdateTime = t
	}}
}

// GetPreconditions create Preconditions
func GetPreconditions(items ...Precondition) Preconditions {
	var result preConditions
	for _, precondition := range items {
		precondition.apply(&result)
	}
	return result
}
