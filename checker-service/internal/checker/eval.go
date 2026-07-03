package checker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/open-policy-agent/opa/v1/rego"
)

func Evaluate(ctx context.Context, policy string, state *gitLabState) (*EvaluateResponse, error) {
	stateJSON, err := json.Marshal(state)
	if err != nil {
		return nil, fmt.Errorf("marshal state: %w", err)
	}

	var input map[string]any
	if err := json.Unmarshal(stateJSON, &input); err != nil {
		return nil, fmt.Errorf("unmarshal input: %w", err)
	}

	r := rego.New(
		rego.Query("data.checker.lab"),
		rego.Module("check.rego", policy),
		rego.Input(input),
	)

	rs, err := r.Eval(ctx)
	if err != nil {
		return nil, fmt.Errorf("rego eval: %w", err)
	}

	resp := &EvaluateResponse{}

	if len(rs) > 0 && len(rs[0].Expressions) > 0 {
		res, ok := rs[0].Expressions[0].Value.(map[string]any)
		if !ok {
			return nil, errors.New("unexpected rego output type")
		}

		if allow, ok := res["allow"].(bool); ok {
			resp.Allow = allow
		}

		if v, ok := res["violations"]; ok {
			for _, msg := range v.([]any) {
				if s, ok := msg.(string); ok {
					resp.Violations = append(resp.Violations, s)
				}
			}
		}
	}

	return resp, nil
}
