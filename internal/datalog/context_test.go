package datalog

import (
	"context"
	"fmt"
	"github.com/stretchr/testify/require"
	"strconv"
	"testing"
	"time"
)

func TestEvaluationCancellationDuringSQLAndRecovery(t *testing.T) {
	db := setupTestDB(t)
	for i := 0; i < 100; i++ {
		addTestFact(t, db, "item", `{"n":`+strconv.Itoa(i)+`}`)
	}
	rule := Rule{Name: "cross", Head: Atom{Rel: "cross", Args: map[string]Term{"a": Var("A"), "b": Var("B"), "c": Var("C"), "d": Var("D")}}, Body: []Atom{
		{Rel: "item", Args: map[string]Term{"n": Var("A")}}, {Rel: "item", Args: map[string]Term{"n": Var("B")}}, {Rel: "item", Args: map[string]Term{"n": Var("C")}}, {Rel: "item", Args: map[string]Term{"n": Var("D")}},
	}}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Millisecond)
	defer cancel()
	rows, err := EvaluateContext(ctx, db, []Rule{rule}, "cross", nil, TemporalScope{}, EvalOptions{})
	require.ErrorIs(t, err, context.DeadlineExceeded)
	require.Nil(t, rows)
	require.ErrorIs(t, ctx.Err(), context.DeadlineExceeded)
	rows, err = Evaluate(db, nil, "item", nil, TemporalScope{})
	require.NoError(t, err)
	require.Len(t, rows, 100)
}

func TestRecursiveCancellationRollsBackTemporaryTables(t *testing.T) {
	db := setupTestDB(t)
	for i := 0; i < 120; i++ {
		addTestFact(t, db, "link", fmt.Sprintf(`{"from":"x%d","to":"x%d"}`, i, i+1))
	}
	rules := mustParse(t, deepReachRules)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Millisecond)
	defer cancel()
	rows, err := EvaluateContext(ctx, db, rules, "reach", nil, TemporalScope{}, EvalOptions{MaxIterations: 200})
	require.ErrorIs(t, err, context.DeadlineExceeded)
	require.Nil(t, rows)
	require.ErrorIs(t, ctx.Err(), context.DeadlineExceeded)
	rows, err = EvaluateWithOptions(db, rules, "reach", map[string]any{"from": "x0"}, TemporalScope{}, EvalOptions{MaxIterations: 200})
	require.NoError(t, err)
	require.Len(t, rows, 120)
}
