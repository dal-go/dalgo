package access

import (
	"context"
	"fmt"
)

// AssessContextPlan assesses the actual immutable request against every policy
// supplied by the existing SecureDB options AND inherited WithPolicy context.
// Database providers are required snapshots exactly as for secured reads; a
// provider failure, empty/nil snapshot or required missing context fails closed.
// All policies must be explicitly safe for inspection. Row/field obligations
// remain in Assessment; allow/conditional is not authority to skip enforcement.
// No DAL/storage operation is executed and no reusable authorization is minted.
// Callers supply the same trusted DB policy configuration used for the operation;
// this function cannot discover policies hidden in an unrelated wrapped handle.
// Request/options/policy implementations must be immutable during this call.
// Existing AssessPlan behavior is unchanged. Coordinator options convey no read
// permission and this call does not inspect protected write/storage evidence.
func AssessContextPlan(ctx context.Context, request Request, options ...DBOption) (Assessment, error) {
	unavailable := func(err error) (Assessment, error) {
		return Assessment{Outcome: AssessmentIndeterminate, Complete: false, Policies: []PolicyAssessment{}, Restrictions: []AssessmentRestriction{}}, err
	}
	if ctx == nil {
		return unavailable(fmt.Errorf("access: assessment context required"))
	}
	if err := ctx.Err(); err != nil {
		return unavailable(err)
	}
	var settings secureDBOptions
	for _, option := range options {
		if option == nil {
			return unavailable(fmt.Errorf("access: nil DB option"))
		}
		if err := option(&settings); err != nil {
			return unavailable(err)
		}
	}
	guard := guard{databasePolicies: append([]Policy(nil), settings.databasePolicies...), policyProvider: settings.policyProvider, requireContext: settings.requireContext}
	policies, err := guard.requestPolicies(ctx, request)
	if err != nil {
		return unavailable(err)
	}
	if err := ctx.Err(); err != nil {
		return unavailable(err)
	}
	assessment := AssessPlan(ctx, request, policies)
	if err := ctx.Err(); err != nil {
		return unavailable(err)
	}
	return assessment, nil
}
