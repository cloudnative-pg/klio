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

package v1alpha1

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestToCoreV1Nil(t *testing.T) {
	var p *PodTemplateSpec
	assert.Nil(t, p.ToCoreV1())
}

func TestToCoreV1LabelsAndAnnotations(t *testing.T) {
	p := &PodTemplateSpec{
		Metadata: EmbeddedObjectMeta{
			Labels:      map[string]string{"app": "klio"},
			Annotations: map[string]string{"note": "test"},
		},
	}

	result := p.ToCoreV1()
	require.NotNil(t, result)
	assert.Equal(t, map[string]string{"app": "klio"}, result.Labels)
	assert.Equal(t, map[string]string{"note": "test"}, result.Annotations)
}

func TestToCoreV1EmptyMetadata(t *testing.T) {
	p := &PodTemplateSpec{}

	result := p.ToCoreV1()
	require.NotNil(t, result)
	assert.Nil(t, result.Labels)
	assert.Nil(t, result.Annotations)
}
