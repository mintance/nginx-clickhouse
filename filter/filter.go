// Package filter provides expression-based filtering and sampling for parsed
// NGINX log entries using github.com/expr-lang/expr.
package filter

import (
	"fmt"
	"math/rand/v2"
	"strconv"

	"github.com/expr-lang/expr"
	"github.com/expr-lang/expr/vm"
	"github.com/sirupsen/logrus"

	"github.com/mintance/nginx-clickhouse/config"
	"github.com/mintance/nginx-clickhouse/nginx"
)

// compiledRule holds a pre-compiled expression and its associated action.
type compiledRule struct {
	program    *vm.Program
	action     string  // "drop" or "keep"
	sampleRate float64 // 0 = disabled (process all matches), 0-1 = fraction
	raw        string  // original expression for logging
}

// Chain is an ordered list of compiled filter rules.
type Chain struct {
	rules  []compiledRule
	fields []string // field names to extract from entries
}

// numericFields are NGINX log fields that should be converted to numeric types
// for expression evaluation (so users can write status >= 500 instead of
// int(status) >= 500).
var numericFields = map[string]string{
	"bytes_sent":             "int",
	"body_bytes_sent":        "int",
	"connections_waiting":    "int",
	"connections_active":     "int",
	"status":                 "int",
	"connection":             "int",
	"request_length":         "int",
	"request_time":           "float",
	"upstream_connect_time":  "float",
	"upstream_header_time":   "float",
	"upstream_response_time": "float",
	"msec":                   "float",
}

// NewChain compiles the filter rules and returns a Chain ready for use.
// The fields parameter lists all field names that will be available in
// expressions (used to build the type environment for the compiler).
func NewChain(rules []config.FilterRule, fields []string) (*Chain, error) {
	if len(rules) == 0 {
		return &Chain{}, nil
	}

	// Build a sample environment for expr type-checking.
	env := buildEnvSample(fields)

	compiled := make([]compiledRule, 0, len(rules))
	for _, r := range rules {
		if r.Action != "drop" && r.Action != "keep" {
			return nil, fmt.Errorf("invalid filter action %q (must be \"drop\" or \"keep\")", r.Action)
		}
		if r.SampleRate < 0 || r.SampleRate > 1 {
			return nil, fmt.Errorf("invalid sample_rate %g for filter %q (must be 0-1)", r.SampleRate, r.Expr)
		}

		program, err := expr.Compile(r.Expr, expr.Env(env), expr.AsBool())
		if err != nil {
			return nil, fmt.Errorf("compile filter %q: %w", r.Expr, err)
		}

		compiled = append(compiled, compiledRule{
			program:    program,
			action:     r.Action,
			sampleRate: r.SampleRate,
			raw:        r.Expr,
		})
	}

	return &Chain{rules: compiled, fields: fields}, nil
}

// Apply evaluates all rules against entries and returns the filtered result.
// Rules are applied in order. For "drop" rules, matching entries are removed.
// For "keep" rules, only matching entries are retained.
// When a rule has a sample_rate, only that fraction of matching entries are
// affected by the action.
func (c *Chain) Apply(entries []nginx.LogEntry) []nginx.LogEntry {
	if len(c.rules) == 0 || len(entries) == 0 {
		return entries
	}

	result := entries
	for _, rule := range c.rules {
		result = applyRule(rule, result, c.fields)
	}
	return result
}

// applyRule filters entries through a single compiled rule.
func applyRule(rule compiledRule, entries []nginx.LogEntry, fields []string) []nginx.LogEntry {
	kept := make([]nginx.LogEntry, 0, len(entries))
	for _, entry := range entries {
		env := buildEnv(entry, fields)
		output, err := expr.Run(rule.program, env)
		if err != nil {
			logrus.WithError(err).WithField("expr", rule.raw).Warn("filter expression error, keeping entry")
			kept = append(kept, entry)
			continue
		}

		matched, ok := output.(bool)
		if !ok {
			kept = append(kept, entry)
			continue
		}

		if !matched {
			// Rule doesn't match this entry — pass through unchanged.
			if rule.action == "keep" {
				// "keep" rule: non-matching entries are dropped.
				continue
			}
			kept = append(kept, entry)
			continue
		}

		// Rule matches. Apply sampling if configured.
		if rule.sampleRate > 0 && rule.sampleRate < 1 && rand.Float64() >= rule.sampleRate {
			// Sampling says skip this match — invert the action.
			if rule.action == "drop" {
				kept = append(kept, entry) // not sampled for drop → keep
			}
			// "keep" with sampling: not sampled → drop
			continue
		}

		// Matched and sampled (or no sampling). Apply action.
		if rule.action == "keep" {
			kept = append(kept, entry)
		}
		// "drop" action: entry is not added to kept.
	}
	return kept
}

// buildEnvSample creates a sample environment map with zero values of the
// correct types for expr compile-time type-checking.
func buildEnvSample(fields []string) map[string]any {
	env := make(map[string]any, len(fields))
	for _, f := range fields {
		switch numericFields[f] {
		case "int":
			env[f] = 0
		case "float":
			env[f] = 0.0
		default:
			env[f] = ""
		}
	}
	return env
}

// buildEnv extracts field values from a LogEntry into a map suitable for
// expr.Run. Numeric fields are converted to their appropriate types.
func buildEnv(entry nginx.LogEntry, fields []string) map[string]any {
	env := make(map[string]any, len(fields))
	for _, f := range fields {
		val, err := entry.Field(f)
		if err != nil {
			// Field not present — use zero value matching the type.
			switch numericFields[f] {
			case "int":
				env[f] = 0
			case "float":
				env[f] = 0.0
			default:
				env[f] = ""
			}
			continue
		}

		switch numericFields[f] {
		case "int":
			n, err := strconv.Atoi(val)
			if err != nil {
				env[f] = 0
			} else {
				env[f] = n
			}
		case "float":
			n, err := strconv.ParseFloat(val, 64)
			if err != nil {
				env[f] = 0.0
			} else {
				env[f] = n
			}
		default:
			env[f] = val
		}
	}
	return env
}
