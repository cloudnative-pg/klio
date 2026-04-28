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
