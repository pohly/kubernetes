/*
Copyright 2022 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package cel

import (
	"context"
	"fmt"
	"reflect"
	"sync"

	"github.com/google/cel-go/cel"
	"github.com/google/cel-go/ext"
	"github.com/google/cel-go/interpreter"

	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/util/version"
	celconfig "k8s.io/apiserver/pkg/apis/cel"
	apiservercel "k8s.io/apiserver/pkg/cel"
	"k8s.io/apiserver/pkg/cel/environment"
	"k8s.io/dynamic-resource-allocation/api"
)

const (
	deviceVar = "device"
)

var (
	lazyCompilerInit sync.Once
	lazyCompiler     *compiler
)

func GetCompiler() *compiler {
	lazyCompilerInit.Do(func() {
		lazyCompiler = newCompiler()
	})
	return lazyCompiler
}

// CompilationResult represents a compiled expression.
type CompilationResult struct {
	Program     cel.Program
	Error       *apiservercel.Error
	Expression  string
	OutputType  *cel.Type
	Environment *cel.Env
}

// Device defines the input values for a CEL selector expression.
//
// Several different helper types are typedefs for this struct which
// implement access to the content in CEL.
type Device struct {
	// Driver gets used as domain for any attribute which does not already
	// have a domain prefix. If set, then it is also made available as a
	// string attribute.
	Driver     string
	Attributes map[string]map[string]api.DeviceAttribute
	Capacity   map[string]map[string]resource.Quantity
}

// rootResolver implements [interpreter.Activation] for the Eval root.
type rootResolver Device

var _ interpreter.Activation = &rootResolver{}

func (r *rootResolver) ResolveName(name string) (any, bool) {
	switch name {
	case deviceVar:
		return (*device)(r), true
	default:
		return nil, false
	}
}

func (r *rootResolver) Parent() interpreter.Activation {
	return nil
}

type compiler struct {
	envset *environment.EnvSet
}

func newCompiler() *compiler {
	return &compiler{envset: mustBuildEnv()}
}

// CompileCELExpression returns a compiled CEL expression. It evaluates to bool.
//
// TODO (https://github.com/kubernetes/kubernetes/issues/125826): validate AST to detect invalid attribute names.
func (c compiler) CompileCELExpression(expression string, envType environment.Type) CompilationResult {
	resultError := func(errorString string, errType apiservercel.ErrorType) CompilationResult {
		return CompilationResult{
			Error: &apiservercel.Error{
				Type:   errType,
				Detail: errorString,
			},
			Expression: expression,
		}
	}

	env, err := c.envset.Env(envType)
	if err != nil {
		return resultError(fmt.Sprintf("unexpected error loading CEL environment: %v", err), apiservercel.ErrorTypeInternal)
	}

	ast, issues := env.Compile(expression)
	if issues != nil {
		return resultError("compilation failed: "+issues.String(), apiservercel.ErrorTypeInvalid)
	}
	expectedReturnType := cel.BoolType
	if ast.OutputType() != expectedReturnType &&
		ast.OutputType() != cel.AnyType {
		return resultError(fmt.Sprintf("must evaluate to %v or the unknown type, not %v", expectedReturnType.String(), ast.OutputType().String()), apiservercel.ErrorTypeInvalid)
	}
	_, err = cel.AstToCheckedExpr(ast)
	if err != nil {
		// should be impossible since env.Compile returned no issues
		return resultError("unexpected compilation error: "+err.Error(), apiservercel.ErrorTypeInternal)
	}
	prog, err := env.Program(ast,
		cel.InterruptCheckFrequency(celconfig.CheckFrequency),
	)
	if err != nil {
		return resultError("program instantiation failed: "+err.Error(), apiservercel.ErrorTypeInternal)
	}
	return CompilationResult{
		Program:     prog,
		Expression:  expression,
		OutputType:  ast.OutputType(),
		Environment: env,
	}
}

var boolType = reflect.TypeOf(true)

func (c CompilationResult) DeviceMatches(ctx context.Context, input *Device) (bool, error) {
	result, _, err := c.Program.ContextEval(ctx, (*rootResolver)(input))
	if err != nil {
		return false, err
	}
	resultAny, err := result.ConvertToNative(boolType)
	if err != nil {
		return false, fmt.Errorf("CEL result of type %s could not be converted to bool: %w", result.Type().TypeName(), err)
	}
	resultBool, ok := resultAny.(bool)
	if !ok {
		return false, fmt.Errorf("CEL native result value should have been a bool, got instead: %T", resultAny)
	}
	return resultBool, nil
}

func mustBuildEnv() *environment.EnvSet {
	envset := environment.MustBaseEnvSet(environment.DefaultCompatibilityVersion(), false /* strictCost */)

	versioned := []environment.VersionedOptions{
		{
			// Feature epoch was actually 1.31, but we artificially set it to 1.0 because these
			// options should always be present.
			//
			// TODO (https://github.com/kubernetes/kubernetes/issues/123687): set this
			// version properly before going to beta.
			IntroducedVersion: version.MajorMinor(1, 0),
			EnvOptions: []cel.EnvOption{
				cel.Variable(deviceVar, deviceType.CelType()),

				SemverLib(),

				// https://pkg.go.dev/github.com/google/cel-go/ext#Bindings
				//
				// This is useful to simplify attribute lookups because the
				// domain only needs to be given once:
				//
				//    cel.bind(dra, device.attributes["dra.example.com"], dra.oneBool && dra.anotherBool)
				ext.Bindings(),
			},
			DeclTypes: []*apiservercel.DeclType{
				deviceType,
			},
		},
	}
	envset, err := envset.Extend(versioned...)
	if err != nil {
		panic(fmt.Errorf("internal error building CEL environment: %w", err))
	}
	return envset
}
