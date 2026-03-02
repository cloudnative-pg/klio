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
