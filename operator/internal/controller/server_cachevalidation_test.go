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
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	kliov1alpha1 "github.com/cloudnative-pg/klio/operator/api/v1alpha1"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func cacheTestPVCTemplate(size string) corev1.PersistentVolumeClaimSpec {
	return corev1.PersistentVolumeClaimSpec{
		AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
		Resources: corev1.VolumeResourceRequirements{
			Requests: corev1.ResourceList{
				corev1.ResourceStorage: resource.MustParse(size),
			},
		},
	}
}

func cacheTestFileSource(secretName, path string) kliov1alpha1.FileSource {
	return kliov1alpha1.FileSource{
		FileReference: &kliov1alpha1.FileReference{
			Volume: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{SecretName: secretName},
			},
			Path: path,
		},
	}
}

func cacheTestServer(name string) *kliov1alpha1.Server {
	return &kliov1alpha1.Server{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "default"},
		Spec: kliov1alpha1.ServerSpec{
			ImageConfiguration: kliov1alpha1.ImageConfiguration{Image: "klio:test"},
			TLSConfiguration: kliov1alpha1.TLSConfiguration{
				TLSSecretName:      "tls-secret",
				ClientCASecretName: "ca-secret",
			},
			Mode: kliov1alpha1.ModeStandard,
			Tier1: &kliov1alpha1.Tier1Configuration{
				Data:              kliov1alpha1.Data{PersistentVolumeClaimTemplate: cacheTestPVCTemplate("1Gi")},
				EncryptionKeyFile: cacheTestFileSource("enc-secret", "encryption-key.age"),
				IdentityFile:      cacheTestFileSource("id-secret", "identity.txt"),
			},
			Queue: &kliov1alpha1.Queue{PersistentVolumeClaimTemplate: cacheTestPVCTemplate("1Gi")},
		},
	}
}

func cacheTestTier2(cache *kliov1alpha1.Cache) *kliov1alpha1.Tier2Configuration {
	return &kliov1alpha1.Tier2Configuration{
		Cache:             cache,
		S3:                &kliov1alpha1.S3Configuration{BucketName: "test-bucket"},
		EncryptionKeyFile: cacheTestFileSource("enc-secret", "encryption-key.age"),
		IdentityFile:      cacheTestFileSource("id-secret", "identity.txt"),
	}
}

var _ = Describe("Server cache validation", func() {
	ctx := context.Background()

	var created []*kliov1alpha1.Server

	create := func(server *kliov1alpha1.Server) error {
		err := k8sClient.Create(ctx, server)
		if err == nil {
			created = append(created, server)
		}

		return err
	}

	AfterEach(func() {
		for _, server := range created {
			Expect(k8sClient.Delete(ctx, server)).To(Succeed())
		}
		created = nil
	})

	It("accepts a tier1 without a dedicated cache volume", func() {
		Expect(create(cacheTestServer("cache-optional-tier1"))).To(Succeed())
	})

	It("accepts a tier2 without a dedicated cache volume when tier1 is configured", func() {
		server := cacheTestServer("cache-optional-tier2")
		server.Spec.Tier2 = cacheTestTier2(nil)
		Expect(create(server)).To(Succeed())
	})

	It("rejects a tier2 without a dedicated cache volume when tier1 is missing", func() {
		server := cacheTestServer("cache-required-tier2")
		server.Spec.Mode = kliov1alpha1.ModeReadOnly
		server.Spec.Tier1 = nil
		server.Spec.Queue = nil
		server.Spec.Tier2 = cacheTestTier2(nil)

		Expect(create(server)).To(MatchError(
			ContainSubstring("tier2.cache is required when tier1 is not configured")))
	})

	It("accepts a read-only server with a dedicated tier2 cache volume", func() {
		server := cacheTestServer("cache-readonly-tier2")
		server.Spec.Mode = kliov1alpha1.ModeReadOnly
		server.Spec.Tier1 = nil
		server.Spec.Queue = nil
		server.Spec.Tier2 = cacheTestTier2(&kliov1alpha1.Cache{
			PersistentVolumeClaimTemplate: cacheTestPVCTemplate("1Gi"),
		})

		Expect(create(server)).To(Succeed())
	})

	// The shrink rules dereference spec.tier{1,2}.cache: they must tolerate a
	// cache that is absent on either side of the update.
	DescribeTable("allows adding and removing cache volumes",
		func(before, after *kliov1alpha1.Cache) {
			server := cacheTestServer(fmt.Sprintf("cache-transition-%t-%t", before == nil, after == nil))
			server.Spec.Tier1.Cache = before
			server.Spec.Tier2 = cacheTestTier2(before)
			Expect(create(server)).To(Succeed())

			server.Spec.Tier1.Cache = after
			server.Spec.Tier2.Cache = after
			Expect(k8sClient.Update(ctx, server)).To(Succeed())
		},
		Entry("adding", nil, &kliov1alpha1.Cache{PersistentVolumeClaimTemplate: cacheTestPVCTemplate("2Gi")}),
		Entry("removing", &kliov1alpha1.Cache{PersistentVolumeClaimTemplate: cacheTestPVCTemplate("2Gi")}, nil),
	)

	It("still refuses to shrink a cache volume that stays configured", func() {
		server := cacheTestServer("cache-shrink")
		server.Spec.Tier1.Cache = &kliov1alpha1.Cache{
			PersistentVolumeClaimTemplate: cacheTestPVCTemplate("2Gi"),
		}
		Expect(create(server)).To(Succeed())

		server.Spec.Tier1.Cache.PersistentVolumeClaimTemplate = cacheTestPVCTemplate("1Gi")
		Expect(k8sClient.Update(ctx, server)).To(MatchError(
			ContainSubstring("tier1.cache PVC size cannot be decreased")))
	})
})
