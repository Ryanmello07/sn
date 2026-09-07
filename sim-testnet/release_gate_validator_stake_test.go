// The release gate retains complete native stake/permit regressions and the
// actual fail-closed production startup edge, not merely standalone helpers.
package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"testing"
)

// Both source groups must remain completely selected by the exact runtime
// client prefix used by the existing launch-critical producer gate.
func TestProducerGatePinsExactBlockRuntimeClientRegressionsNativeStakeCoverage(t *testing.T) {
	for _, path := range []string{"../crv4/validator_stake_test.go", "../validator/release_native_validator_test.go"} {
		source, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if err := verifyReleaseSourceTestCoverage(releaseRuntimeClientSelector, "^Test", []string{string(source)}); err != nil {
			t.Fatalf("native stake coverage %s: %v", path, err)
		}
	}
}

// The real RunRelease path must test the signing hotkey after EVM registration
// and return on refusal before recovery, state loading or any worker launch.
// Private RPC fixture authorization never appears at this production edge.
func TestProducerGatePinsExactBlockRuntimeClientRegressionsNativeStakeStartup(t *testing.T) {
	file, err := parser.ParseFile(token.NewFileSet(), "../validator/release_run.go", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	var run *ast.FuncDecl
	for _, declaration := range file.Decls {
		if function, ok := declaration.(*ast.FuncDecl); ok && function.Name.Name == "RunRelease" {
			run = function
		}
	}
	if run == nil || run.Body == nil {
		t.Fatal("real RunRelease body is absent")
	}
	positions := map[string][]token.Pos{}
	ast.Inspect(run.Body, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		name := ""
		switch callee := call.Fun.(type) {
		case *ast.Ident:
			name = callee.Name
		case *ast.SelectorExpr:
			name = callee.Sel.Name
		}
		positions[name] = append(positions[name], call.Pos())
		return true
	})
	registration, stake := positions["FindUidByHotkeyAtHashContext"], positions["authenticateReleaseValidatorStakeContext"]
	if len(registration) != 1 || len(stake) != 1 || registration[0] >= stake[0] {
		t.Fatal("native startup stake admission is absent, duplicated or before EVM registration")
	}
	for _, operation := range []string{"RecoverAttemptSettlementEpoch", "loadReleaseAttemptState", "advanceSettlement", "startReleaseOperator", "runReleaseOperatorWorkers"} {
		if len(positions[operation]) == 0 {
			t.Fatalf("startup operation %s disappeared without updating the custody guard", operation)
		}
		for _, position := range positions[operation] {
			if position <= stake[0] {
				t.Fatalf("startup operation %s precedes native stake admission", operation)
			}
		}
	}
	guarded := 0
	for _, statement := range run.Body.List {
		conditional, ok := statement.(*ast.IfStmt)
		if !ok {
			continue
		}
		assignment, ok := conditional.Init.(*ast.AssignStmt)
		if !ok || len(assignment.Lhs) != 2 || len(assignment.Rhs) != 1 {
			continue
		}
		call, ok := assignment.Rhs[0].(*ast.CallExpr)
		if !ok || len(call.Args) != 5 {
			continue
		}
		callee, ok := call.Fun.(*ast.Ident)
		if !ok || callee.Name != "authenticateReleaseValidatorStakeContext" {
			continue
		}
		for _, expected := range []struct {
			index int
			name  string
		}{{index: 0, name: "ctx"}, {index: 1, name: "native"}, {index: 2, name: "cfg"}, {index: 4, name: "validatorUID"}} {
			argument, ok := call.Args[expected.index].(*ast.Ident)
			if !ok || argument.Name != expected.name {
				t.Fatalf("native startup argument %d is not %s", expected.index, expected.name)
			}
		}
		keyCall, ok := call.Args[3].(*ast.CallExpr)
		if !ok || len(keyCall.Args) != 0 {
			t.Fatal("native startup did not use the actual signing public key")
		}
		keyMethod, ok := keyCall.Fun.(*ast.SelectorExpr)
		if !ok || keyMethod.Sel.Name != "PublicKey" {
			t.Fatal("native startup signing method changed")
		}
		keyOwner, ok := keyMethod.X.(*ast.Ident)
		if !ok || keyOwner.Name != "hotkey" {
			t.Fatal("native startup signing owner changed")
		}
		condition, ok := conditional.Cond.(*ast.BinaryExpr)
		if !ok || condition.Op != token.NEQ {
			t.Fatal("native startup error is not tested")
		}
		left, leftOK := condition.X.(*ast.Ident)
		right, rightOK := condition.Y.(*ast.Ident)
		if !leftOK || left.Name != "err" || !rightOK || right.Name != "nil" {
			t.Fatal("native startup condition does not reject err != nil")
		}
		if len(conditional.Body.List) != 1 {
			t.Fatal("native startup refusal does not return directly")
		}
		refusal, ok := conditional.Body.List[0].(*ast.ReturnStmt)
		if !ok || len(refusal.Results) != 1 {
			t.Fatal("native startup refusal is not returned")
		}
		returned, ok := refusal.Results[0].(*ast.Ident)
		if !ok || returned.Name != "err" {
			t.Fatal("native startup refusal is discarded")
		}
		guarded++
	}
	if guarded != 1 {
		t.Fatal("real RunRelease lacks exactly one fail-closed native stake admission")
	}
}
