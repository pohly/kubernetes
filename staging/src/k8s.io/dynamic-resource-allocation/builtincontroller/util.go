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

package builtincontroller

import (
	"context"

	resourcev1alpha2 "k8s.io/api/resource/v1alpha2"
)

// FindClaimController checks one controller after the other and returns
// the first error or claim controller that it encounters.
// The class is non-nil if (and only if) the claim is not allocated yet.
func FindClaimController(ctx context.Context, controllers []ActiveController, claim *resourcev1alpha2.ResourceClaim, class *resourcev1alpha2.ResourceClass) (ClaimController, error) {
	for _, controller := range controllers {
		claimController, err := controller.HandlesClaim(ctx, claim, class)
		if err != nil {
			return nil, err
		}
		if claimController != nil {
			return claimController, nil
		}
	}
	return nil, nil
}
