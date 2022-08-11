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
	"strings"

	"k8s.io/apimachinery/pkg/util/validation"
	"k8s.io/apimachinery/pkg/util/validation/field"
)

// ValidateCDIDriverName checks that a string meets the requirements for the
// ResourceClass.DriverName field. The requirements are the same as for a CSI
// driver name.
func ValidateDriverName(driverName string, fldPath *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}

	if len(driverName) == 0 {
		allErrs = append(allErrs, field.Required(fldPath, ""))
	}

	// TODO: should this length limit be documented with a constant in the core/v1 API?
	if len(driverName) > 63 {
		allErrs = append(allErrs, field.TooLong(fldPath, driverName, 63))
	}

	for _, msg := range validation.IsDNS1123Subdomain(strings.ToLower(driverName)) {
		allErrs = append(allErrs, field.Invalid(fldPath, driverName, msg))
	}

	return allErrs
}
