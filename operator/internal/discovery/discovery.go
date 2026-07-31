/*
Copyright © contributors to CloudNativePG, established as
CloudNativePG a Series of LF Projects, LLC.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.

SPDX-License-Identifier: Apache-2.0
*/

// Package discovery provides cluster capability detection utilities.
package discovery

import (
	"slices"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/discovery"
)

// HaveSecurityContextConstraints queries the discovery API to determine
// whether the cluster implements OpenShift Security Context Constraints.
func HaveSecurityContextConstraints(client discovery.DiscoveryInterface) (bool, error) {
	apiResourceList, err := client.ServerResourcesForGroupVersion("security.openshift.io/v1")
	if err != nil {
		if apierrors.IsNotFound(err) {
			return false, nil
		}

		return false, err
	}

	return slices.ContainsFunc(apiResourceList.APIResources, func(r metav1.APIResource) bool {
		return r.Name == "securitycontextconstraints"
	}), nil
}
