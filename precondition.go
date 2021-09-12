package dalgo

import "time"

type Precondition = func(preconditions *preConditions)

type Preconditions interface {
	Exists() bool
}

type preConditions struct {
	exists         bool
	lastUpdateTime time.Time
}

func (v preConditions) Exists() bool {
	return v.exists
}

func WithExistsPrecondition() func(preconditions *preConditions) {
	return func(preconditions *preConditions) {
		preconditions.exists = true
	}
}

func WithLastUpdateTimePrecondition(t time.Time) func(preconditions *preConditions) {
	return func(preconditions *preConditions) {
		preconditions.lastUpdateTime = t
	}
}

func GetPreconditions(items ...Precondition) Preconditions {
	var result preConditions
	for _, precondition := range items {
		precondition(&result)
	}
	return result
}
