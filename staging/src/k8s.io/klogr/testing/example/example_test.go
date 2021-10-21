/*
Copyright 2021 The Kubernetes Authors.

SPDX-License-Identifier: Apache-2.0
*/

package example

import (
	"fmt"
	"testing"

	"k8s.io/klogr"
	"k8s.io/klogr/internal/test"
	ktesting "k8s.io/klogr/testing"
)

func TestKlogr(t *testing.T) {
	log := ktesting.New(t)
	exampleOutput(log)
}

type pair struct {
	a, b int
}

func (p pair) String() string {
	return fmt.Sprintf("(%d, %d)", p.a, p.b)
}

var _ fmt.Stringer = pair{}

type err struct {
	msg string
}

func (e err) Error() string {
	return "failed: " + e.msg
}

var _ error = err{}

func exampleOutput(log klogr.Logger) {
	log.Info("hello world")
	log.Error(err{msg: "some error"}, "failed")
	log.V(1).Info("verbosity 1")
	log.WithName("main").WithName("helper").Info("with prefix")
	obj := test.KMetadataMock{Name: "joe", NS: "kube-system"}
	log.Info("key/value pairs",
		"int", 1,
		"float", 2.0,
		"pair", pair{a: 1, b: 2},
		"raw", obj,
		"kobj", klogr.KObj(obj),
	)
}
