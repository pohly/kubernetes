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

package validation

import (
	"fmt"

	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"k8s.io/apiserver/pkg/cel"
	"k8s.io/apiserver/pkg/cel/environment"
	corevalidation "k8s.io/kubernetes/pkg/apis/core/validation"
	"k8s.io/kubernetes/pkg/apis/resource/structured/namedresources"
	namedresourcescel "k8s.io/kubernetes/pkg/apis/resource/structured/namedresources/cel"
)

var (
	validateInstanceName  = corevalidation.ValidateDNS1123Subdomain
	validateAttributeName = corevalidation.ValidateDNS1123Subdomain
)

type Options struct {
	// StoredExpressions must be true if and only if validating CEL
	// expressions that were already stored persistently. This makes
	// validation more permissive by enabling CEL definitions that are not
	// valid yet for new expressions.
	StoredExpressions bool
}

func ValidateResources(resources *namedresources.Resources, fldPath *field.Path) field.ErrorList {
	var allErrs field.ErrorList
	for i, instance := range resources.Instances {
		idxPath := fldPath.Index(i)
		allErrs = append(allErrs, validateInstance(instance, idxPath)...)
	}
	return allErrs
}

func validateInstance(instance namedresources.Instance, fldPath *field.Path) field.ErrorList {
	allErrs := validateInstanceName(instance.Name, fldPath.Child("name"))
	allErrs = append(allErrs, validateAttributes(instance.Attributes, fldPath.Child("attributes"))...)
	return allErrs
}

func validateAttributes(attributes []namedresources.Attribute, fldPath *field.Path) field.ErrorList {
	var allErrs field.ErrorList
	for i, attribute := range attributes {
		idxPath := fldPath.Index(i)
		allErrs = append(allErrs, validateAttributeName(attribute.Name, idxPath.Child("name"))...)

		entries := sets.New[string]()
		if attribute.QuantityValue != nil {
			entries.Insert("quantityValue")
		}
		if attribute.BoolValue != nil {
			entries.Insert("boolValue")
		}
		if attribute.IntValue != nil {
			entries.Insert("intValue")
		}
		if attribute.IntSliceValue != nil {
			entries.Insert("intSliceValue")
		}
		if attribute.StringValue != nil {
			entries.Insert("stringValue")
		}
		if attribute.StringSliceValue != nil {
			entries.Insert("stringSliceValue")
		}
		// TODO: VersionValue

		switch len(entries) {
		case 0:
			allErrs = append(allErrs, field.Required(idxPath, "exactly one value must be set"))
		case 1:
			// Okay.
		default:
			allErrs = append(allErrs, field.Invalid(idxPath, sets.List(entries), "exactly one field must be set, not several"))
		}
	}
	return allErrs
}

func ValidateRequest(opts Options, request *namedresources.Request, fldPath *field.Path) field.ErrorList {
	return validateSelector(opts, request.Selector, fldPath.Child("selector"))
}

func ValidateFilter(opts Options, filter *namedresources.Filter, fldPath *field.Path) field.ErrorList {
	return validateSelector(opts, filter.Selector, fldPath.Child("selector"))
}

func validateSelector(opts Options, selector string, fldPath *field.Path) field.ErrorList {
	var allErrs field.ErrorList
	if selector == "" {
		allErrs = append(allErrs, field.Required(fldPath, ""))
	} else {
		envType := environment.NewExpressions
		if opts.StoredExpressions {
			envType = environment.StoredExpressions
		}
		result := namedresourcescel.Compiler.CompileCELExpression(selector, envType)
		if result.Error != nil {
			allErrs = append(allErrs, convertCELErrorToValidationError(fldPath, selector, result.Error))
		}
	}
	return allErrs
}

func convertCELErrorToValidationError(fldPath *field.Path, expression string, err *cel.Error) *field.Error {
	switch err.Type {
	case cel.ErrorTypeRequired:
		return field.Required(fldPath, err.Detail)
	case cel.ErrorTypeInvalid:
		return field.Invalid(fldPath, expression, err.Detail)
	case cel.ErrorTypeInternal:
		return field.InternalError(fldPath, err)
	}
	return field.InternalError(fldPath, fmt.Errorf("unsupported error type: %w", err))
}

func ValidateAllocationResult(result *namedresources.AllocationResult, fldPath *field.Path) field.ErrorList {
	var allErrs field.ErrorList
	for i, instance := range result.Instances {
		idxPath := fldPath.Index(i)
		allErrs = append(allErrs, validateAllocatedInstance(instance, idxPath)...)
	}
	return allErrs
}

func validateAllocatedInstance(instance namedresources.AllocatedInstance, fldPath *field.Path) field.ErrorList {
	return validateInstanceName(instance.Name, fldPath.Child("name"))
}
