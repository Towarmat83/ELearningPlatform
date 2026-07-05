// Package checker evaluates lab submissions against GitLab state via OPA.
package checker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/open-policy-agent/opa/v1/rego"
)

// Evaluate runs the given Rego policy against the GitLab state and returns
// the resulting allow/violations verdict.
func Evaluate(ctx context.Context, policy string, state *gitLabState) (*EvaluateResponse, error) {
	stateJSON, err := json.Marshal(state)
	if err != nil {
		return nil, fmt.Errorf("marshal state: %w", err)
	}

	var input map[string]any

	err = json.Unmarshal(stateJSON, &input)
	if err != nil {
		return nil, fmt.Errorf("unmarshal input: %w", err)
	}

	regoQuery := rego.New(
		rego.Query("data.checker.lab"),
		rego.Module("check.rego", policy),
		rego.Input(input),
	)

	resultSet, err := regoQuery.Eval(ctx)
	if err != nil {
		return nil, fmt.Errorf("rego eval: %w", err)
	}

	resp := &EvaluateResponse{}

	expr, found := firstExpression(resultSet)
	if !found {
		return resp, nil
	}

	res, ok := expr.(map[string]any)
	if !ok {
		return nil, errors.New("unexpected rego output type")
	}

	if allow, ok := res["allow"].(bool); ok {
		resp.Allow = allow
	}

	resp.Violations = extractViolations(res["violations"])

	return resp, nil
}

// firstExpression returns the first expression value of the first rego
// result, if present.
func firstExpression(resultSet rego.ResultSet) (any, bool) {
	if len(resultSet) == 0 || len(resultSet[0].Expressions) == 0 {
		return nil, false
	}

	return resultSet[0].Expressions[0].Value, true
}

// extractViolations converts a raw violations value into a []string,
// skipping any non-string entries.
func extractViolations(raw any) []string {
	list, ok := raw.([]any)
	if !ok {
		return nil
	}

	violations := make([]string, 0, len(list))

	for _, msg := range list {
		if s, ok := msg.(string); ok {
			violations = append(violations, s)
		}
	}

	return violations
}
