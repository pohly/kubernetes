/*
Copyright 2023 The Kubernetes Authors.

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

package meta

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type LabelSelectorOperator = metav1.LabelSelectorOperator

// This list of operators is part of the k8s.io/dynamic-resource-allocation
// meta v1 API. Adding new operators is an API change that must be handled
// with care!
const (
	LabelSelectorOpIn           = metav1.LabelSelectorOpIn
	LabelSelectorOpNotIn        = metav1.LabelSelectorOpNotIn
	LabelSelectorOpExists       = metav1.LabelSelectorOpExists
	LabelSelectorOpDoesNotExist = metav1.LabelSelectorOpDoesNotExist
	LabelSelectorOpVersionGE    = LabelSelectorOperator("Version>=")
	LabelSelectorOpVersionGT    = LabelSelectorOperator("Version>")
	LabelSelectorOpVersionLT    = LabelSelectorOperator("Version<")
	LabelSelectorOpVersionLE    = LabelSelectorOperator("Version<=")
	LabelSelectorOpVersionEQ    = LabelSelectorOperator("Version=")
	LabelSelectorOpVersionNE    = LabelSelectorOperator("Version!=")
	LabelSelectorOpGE           = LabelSelectorOperator(">=")
	LabelSelectorOpGT           = LabelSelectorOperator(">")
	LabelSelectorOpLT           = LabelSelectorOperator("<")
	LabelSelectorOpLE           = LabelSelectorOperator("<=")
	LabelSelectorOpEQ           = LabelSelectorOperator("=")
	LabelSelectorOpNE           = LabelSelectorOperator("!=")
)
