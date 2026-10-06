/*
Copyright The Kubernetes Authors.

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

package legacyscheme

import (
	"io"
	"testing"

	"k8s.io/apimachinery/pkg/conversion"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/klog/v2"
)

// finalizeTestObject is a minimal runtime.Object usable with AddKnownTypeWithName.
type finalizeTestObject struct {
	runtime.TypeMeta
}

func (o *finalizeTestObject) DeepCopyObject() runtime.Object {
	out := *o
	return &out
}

// finalizeTestSource/Dest let the test register a conversion func that
// isn't one of NewScheme's built-in default conversions.
type finalizeTestSource int
type finalizeTestDest int

// TestFinalize covers the behavior that makes Finalize safe to call more
// than once: Scheme, Codecs, and ParameterCodec all get replaced with values
// derived from baseScheme and the init funcs' current effect, and baseScheme
// itself is never mutated. The same checks run both before and after
// flipping enabled, so each one is exercised in both states.
func TestFinalize(t *testing.T) {
	gvk := schema.GroupVersionKind{Group: "legacyscheme.test", Version: "v1", Kind: "FinalizeTestObject"}
	var enabled bool
	baseScheme.AddInitFunc(func(logger klog.Logger, scheme *runtime.Scheme) error {
		if enabled {
			scheme.AddKnownTypeWithName(gvk, &finalizeTestObject{})
		}
		return nil
	})

	// Registered once on baseScheme; every Finalize call must carry it over
	// into the clone it returns (Converter's own Clone correctness is
	// covered in detail by the conversion package's own tests).
	if err := baseScheme.Converter().RegisterUntypedConversionFunc(
		(*finalizeTestSource)(nil), (*finalizeTestDest)(nil),
		func(a, b interface{}, s conversion.Scope) error {
			*b.(*finalizeTestDest) = finalizeTestDest(*a.(*finalizeTestSource))
			return nil
		},
	); err != nil {
		t.Fatal(err)
	}

	check := func(label string, wantRegistered bool) {
		t.Helper()
		if got := Scheme.Recognizes(gvk); got != wantRegistered {
			t.Errorf("%s: Scheme.Recognizes = %v, want %v", label, got, wantRegistered)
		}
		if baseScheme.Recognizes(gvk) {
			t.Errorf("%s: baseScheme itself must never be mutated by init funcs", label)
		}
		err := Codecs.LegacyCodec(gvk.GroupVersion()).Encode(&finalizeTestObject{}, io.Discard)
		if wantRegistered && err != nil {
			t.Errorf("%s: Codecs must reflect the current Scheme: %v", label, err)
		}
		if !wantRegistered && err == nil {
			t.Errorf("%s: expected an error encoding a type that is not registered", label)
		}
		if ParameterCodec == nil {
			t.Errorf("%s: ParameterCodec must be set", label)
		}
		var src finalizeTestSource = 42
		var dst finalizeTestDest
		if err := Scheme.Convert(&src, &dst, nil); err != nil {
			t.Errorf("%s: Scheme must carry over conversion funcs registered on baseScheme: %v", label, err)
		} else if dst != 42 {
			t.Errorf("%s: conversion func did not run, got %v", label, dst)
		}
	}

	if err := Finalize(klog.Background()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	check("disabled", false)

	prevScheme := Scheme
	enabled = true
	if err := Finalize(klog.Background()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if Scheme == prevScheme {
		t.Error("Finalize must replace Scheme with a new instance, not reuse the old one")
	}
	check("enabled", true)
	if prevScheme.Recognizes(gvk) {
		t.Error("the previous Scheme instance must not retroactively change")
	}
}
