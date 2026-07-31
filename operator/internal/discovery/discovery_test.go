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

package discovery

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	fakediscovery "k8s.io/client-go/discovery/fake"
	"k8s.io/client-go/kubernetes/fake"
)

func TestHaveSecurityContextConstraints(t *testing.T) {
	t.Run("returns false on vanilla Kubernetes", func(t *testing.T) {
		client := fake.NewClientset()
		got, err := HaveSecurityContextConstraints(client.Discovery())
		require.NoError(t, err)
		assert.False(t, got)
	})

	t.Run("returns true on OpenShift", func(t *testing.T) {
		client := fake.NewClientset()
		fakeDiscovery, ok := client.Discovery().(*fakediscovery.FakeDiscovery)
		require.True(t, ok)
		fakeDiscovery.Resources = []*metav1.APIResourceList{
			{
				GroupVersion: "security.openshift.io/v1",
				APIResources: []metav1.APIResource{
					{Name: "securitycontextconstraints"},
				},
			},
		}
		got, err := HaveSecurityContextConstraints(client.Discovery())
		require.NoError(t, err)
		assert.True(t, got)
	})

	t.Run("returns false when SCC resource is absent from the group", func(t *testing.T) {
		client := fake.NewClientset()
		fakeDiscovery, ok := client.Discovery().(*fakediscovery.FakeDiscovery)
		require.True(t, ok)
		fakeDiscovery.Resources = []*metav1.APIResourceList{
			{
				GroupVersion: "security.openshift.io/v1",
				APIResources: []metav1.APIResource{
					{Name: "other"},
				},
			},
		}
		got, err := HaveSecurityContextConstraints(client.Discovery())
		require.NoError(t, err)
		assert.False(t, got)
	})
}
