package main

// The durable shutdown tests use the actual RunRelease owner operation; these
// guards pin its returned error and the real final-save callback route.

import (
	"go/ast"
	"go/parser"
	"go/token"
	"testing"
)

func TestReleaseShutdownPublicRuntimeReturnsOwnedWorkerResult(t *testing.T) {
	parsed, err := parser.ParseFile(token.NewFileSet(), "../validator/release_run.go", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	matched, diskCloseJoined := false, false
	for _, declaration := range parsed.Decls {
		function, ok := declaration.(*ast.FuncDecl)
		if !ok || function.Name.Name != "RunRelease" || function.Body == nil {
			continue
		}
		ast.Inspect(function.Body, func(node ast.Node) bool {
			deferred, ok := node.(*ast.DeferStmt)
			if !ok {
				return true
			}
			ast.Inspect(deferred.Call, func(node ast.Node) bool {
				assignment, ok := node.(*ast.AssignStmt)
				if !ok || assignment.Tok != token.ASSIGN || len(assignment.Lhs) != 1 || len(assignment.Rhs) != 1 {
					return true
				}
				name, ok := assignment.Lhs[0].(*ast.Ident)
				if !ok || name.Name != "returnErr" {
					return true
				}
				join, ok := assignment.Rhs[0].(*ast.CallExpr)
				if !ok || len(join.Args) != 2 {
					return true
				}
				target, ok := join.Fun.(*ast.SelectorExpr)
				if !ok || target.Sel.Name != "Join" {
					return true
				}
				owner, ok := target.X.(*ast.Ident)
				if !ok || owner.Name != "errors" {
					return true
				}
				previous, ok := join.Args[0].(*ast.Ident)
				if !ok || previous.Name != "returnErr" {
					return true
				}
				closeCall, ok := join.Args[1].(*ast.CallExpr)
				if !ok || len(closeCall.Args) != 0 {
					return true
				}
				closeTarget, ok := closeCall.Fun.(*ast.SelectorExpr)
				if !ok || closeTarget.Sel.Name != "close" {
					return true
				}
				disk, ok := closeTarget.X.(*ast.Ident)
				diskCloseJoined = diskCloseJoined || ok && disk.Name == "disk"
				return true
			})
			return false
		})
		for _, statement := range function.Body.List {
			result, ok := statement.(*ast.ReturnStmt)
			if !ok || len(result.Results) != 1 {
				continue
			}
			call, ok := result.Results[0].(*ast.CallExpr)
			if !ok {
				continue
			}
			name, ok := call.Fun.(*ast.Ident)
			if ok && name.Name == "runReleaseOperatorWorkers" {
				matched = true
			}
		}
	}
	if !matched {
		t.Fatal("public release runtime does not return the actual owned worker lifecycle result")
	}
	if !diskCloseJoined {
		t.Fatal("public release runtime does not join its deferred disk owner close into the returned error")
	}
	for _, check := range []struct{ path, function, callee string }{
		{path: "../validator/release_run.go", function: "startReleaseOperatorWithAdmission", callee: "newReleaseOperatorClose"},
		{path: "../validator/release_shutdown.go", function: "newReleaseOperatorClose", callee: "Save"},
		{path: "../validator/release_shutdown.go", function: "newReleaseOperatorClose", callee: "closeResources"},
		{path: "../validator/release_shutdown.go", function: "runReleaseOperatorWorkers", callee: "reportReleaseTrailEngineError"},
		{path: "../validator/release_shutdown.go", function: "runReleaseOperatorWorkers", callee: "Wait"},
		{path: "../validator/release_shutdown.go", function: "runReleaseOperatorWorkers", callee: "close"},
		{path: "../validator/release_steer.go", function: "Run", callee: "runReleaseSteeringLoop"},
		{path: "../validator/release_steer.go", function: "Run", callee: "SubmitOnce"},
		{path: "../validator/release_steer.go", function: "runReleaseSteeringLoop", callee: "runReleaseSteeringLoopWithWait"},
		{path: "../validator/release_steer.go", function: "SubmitOnce", callee: "recordReleasePendingError"},
		{path: "../validator/release_steer.go", function: "reconcilePending", callee: "recordReleasePendingError"},
		{path: "../validator/release_run.go", function: "RunRelease", callee: "openReleaseEvidenceV2DiskState"},
		{path: "../validator/release_state_v2.go", function: "openReleaseEvidenceV2DiskState", callee: "openReleaseEvidenceV2DiskStateWithObserver"},
		{path: "../validator/release_state_v2.go", function: "openReleaseEvidenceV2DiskStateWithObserver", callee: "close"},
		{path: "../validator/release_state_v2.go", function: "close", callee: "Close"},
		{path: "../validator/release_shutdown.go", function: "closeReleaseAttemptStates", callee: "Close"},
		{path: "../validator/release_run.go", function: "loadReleaseAttemptStateWithObserver", callee: "Close"},
		{path: "../validator/intent.go", function: "Begin", callee: "validateSteeringIntentLifecycle"},
		{path: "../validator/intent.go", function: "update", callee: "validateSteeringIntentLifecycle"},
	} {
		if !releaseClosureFunctionCalls(t, check.path, check.function)[check.callee] {
			t.Errorf("%s omits actual shutdown call %s", check.function, check.callee)
		}
	}
	if releaseClosureFunctionCalls(t, "../validator/release_shutdown_inner.go", "recordReleasePendingError")["update"] {
		t.Fatal("uncertain pending error handler rewrites the restart authority")
	}
}
