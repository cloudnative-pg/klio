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

package controller

import (
	"fmt"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type incorrectOwnershipError struct {
	ActualController   corev1.TypedObjectReference
	ExpectedController corev1.TypedObjectReference
	Object             corev1.TypedObjectReference
}

func newIncorrectOwnershipError(
	object client.Object,
	expectedOwner client.Object,
) *incorrectOwnershipError {
	objectToReference := func(o client.Object) corev1.TypedObjectReference {
		namespace := o.GetNamespace()
		group := o.GetObjectKind().GroupVersionKind().Group

		return corev1.TypedObjectReference{
			Name:      o.GetName(),
			Namespace: &namespace,
			Kind:      o.GetObjectKind().GroupVersionKind().Kind,
			APIGroup:  &group,
		}
	}

	ownerToReference := func(o *metav1.OwnerReference) corev1.TypedObjectReference {
		namespace := object.GetNamespace()

		if o == nil {
			return corev1.TypedObjectReference{}
		}

		return corev1.TypedObjectReference{
			Name:      o.Name,
			Namespace: &namespace,
			Kind:      o.Kind,
			APIGroup:  &o.APIVersion,
		}
	}

	return &incorrectOwnershipError{
		Object:             objectToReference(object),
		ActualController:   ownerToReference(metav1.GetControllerOf(object)),
		ExpectedController: objectToReference(expectedOwner),
	}
}

func (e *incorrectOwnershipError) Error() string {
	return fmt.Sprintf(
		"expected %s to be owned by %s and not by %s",
		e.Object.String(),
		e.ExpectedController.String(),
		e.ActualController.String(),
	)
}
