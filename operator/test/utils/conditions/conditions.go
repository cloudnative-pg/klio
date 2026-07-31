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

package conditions

import (
	"context"
	"slices"

	certmanagerv1 "github.com/cert-manager/cert-manager/pkg/apis/certmanager/v1"
	cmmeta "github.com/cert-manager/cert-manager/pkg/apis/meta/v1"
	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"sigs.k8s.io/e2e-framework/klient/k8s"
	"sigs.k8s.io/e2e-framework/klient/k8s/resources"
	waitConditions "sigs.k8s.io/e2e-framework/klient/wait/conditions"
)

// KlioServerIsReady checks if the given KlioServer is ready by checking the readiness of its pod.
func KlioServerIsReady(r *resources.Resources, server k8s.Object) wait.ConditionWithContextFunc {
	// TODO: This is a temporary solution, we should use the KlioServer controller to manage the readiness of the server.
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      server.GetName() + "-klio-0",
			Namespace: server.GetNamespace(),
		},
	}

	return waitConditions.New(r).PodReady(pod)
}

// IssuerIsReady checks if the given Issuer is ready.
func IssuerIsReady(r *resources.Resources, issuer *certmanagerv1.Issuer) wait.ConditionWithContextFunc {
	return func(ctx context.Context) (bool, error) {
		currentIssuer := &certmanagerv1.Issuer{}
		if err := r.Get(ctx, issuer.Name, issuer.Namespace, currentIssuer); err != nil {
			return false, err
		}

		return slices.ContainsFunc(currentIssuer.Status.Conditions, func(c certmanagerv1.IssuerCondition) bool {
			return c.Type == certmanagerv1.IssuerConditionReady && c.Status == cmmeta.ConditionTrue
		}), nil
	}
}

// CertificateIsReady checks if the given Certificate is ready.
func CertificateIsReady(r *resources.Resources, cert *certmanagerv1.Certificate) wait.ConditionWithContextFunc {
	return func(ctx context.Context) (bool, error) {
		currentCert := &certmanagerv1.Certificate{}
		if err := r.Get(ctx, cert.Name, cert.Namespace, currentCert); err != nil {
			return false, err
		}

		return slices.ContainsFunc(currentCert.Status.Conditions, func(c certmanagerv1.CertificateCondition) bool {
			return c.Type == certmanagerv1.CertificateConditionReady && c.Status == cmmeta.ConditionTrue
		}), nil
	}
}

// DeploymentIsReady checks if the given Deployment is ready.
func DeploymentIsReady(r *resources.Resources, deployment *appsv1.Deployment) wait.ConditionWithContextFunc {
	return waitConditions.New(r).DeploymentConditionMatch(deployment, appsv1.DeploymentAvailable, corev1.ConditionTrue)
}

// JobIsComplete checks if the given Job is complete.
func JobIsComplete(r *resources.Resources, job *batchv1.Job) wait.ConditionWithContextFunc {
	return func(ctx context.Context) (bool, error) {
		currentJob := &batchv1.Job{}
		if err := r.Get(ctx, job.Name, job.Namespace, currentJob); err != nil {
			return false, err
		}

		return slices.ContainsFunc(currentJob.Status.Conditions, func(c batchv1.JobCondition) bool {
			return c.Type == batchv1.JobComplete && c.Status == corev1.ConditionTrue
		}), nil
	}
}
