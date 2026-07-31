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

package cnpgi

import (
	"context"

	"github.com/cloudnative-pg/cnpg-i-machinery/pkg/pluginhelper/http"
	"github.com/cloudnative-pg/cnpg-i/pkg/lifecycle"
	"github.com/cloudnative-pg/cnpg-i/pkg/reconciler"
	"google.golang.org/grpc"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// CNPGI implementation for the operator.
type CNPGI struct {
	Client                         client.Client
	ServerCertPath                 string
	ServerKeyPath                  string
	ClientCertPath                 string
	ServerAddress                  string
	HaveSecurityContextConstraints bool
}

// Start the GRPC server of the operator.
func (c *CNPGI) Start(ctx context.Context) error {
	enrich := func(server *grpc.Server) error {
		reconciler.RegisterReconcilerHooksServer(server, ReconcilerImplementation{
			Client: c.Client,
		})
		lifecycle.RegisterOperatorLifecycleServer(server, LifecycleImplementation{
			Client:                         c.Client,
			HaveSecurityContextConstraints: c.HaveSecurityContextConstraints,
		})

		return nil
	}

	srv := http.Server{
		IdentityImpl:   IdentityImplementation{},
		Enrichers:      []http.ServerEnricher{enrich},
		ServerCertPath: c.ServerCertPath,
		ServerKeyPath:  c.ServerKeyPath,
		ClientCertPath: c.ClientCertPath,
		ServerAddress:  c.ServerAddress,
	}

	return srv.Start(ctx)
}
