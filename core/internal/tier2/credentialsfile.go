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

package tier2

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
)

// credentialsFileTTL is how long the AWS SDK caches the credentials read from
// the shared credentials file before reading it again.
const credentialsFileTTL = time.Minute

// ErrCredentialsFileNoKeys is returned when the selected profile of the
// shared credentials file does not contain any access key.
var ErrCredentialsFileNoKeys = errors.New("no access keys in the shared credentials file profile")

// credentialsFileProvider reads the AWS shared credentials file each time the
// SDK asks for credentials, so a rotated file is picked up without a restart.
type credentialsFileProvider struct {
	path    string
	profile string
}

// Retrieve implements aws.CredentialsProvider.
func (p credentialsFileProvider) Retrieve(ctx context.Context) (aws.Credentials, error) {
	profile := p.profile
	if profile == "" {
		profile = awsconfig.DefaultSharedConfigProfile
	}

	shared, err := awsconfig.LoadSharedConfigProfile(ctx, profile, func(o *awsconfig.LoadSharedConfigOptions) {
		o.CredentialsFiles = []string{p.path}
		o.ConfigFiles = []string{}
	})
	if err != nil {
		return aws.Credentials{}, fmt.Errorf("while reading the shared credentials file: %w", err)
	}

	creds := shared.Credentials
	if !creds.HasKeys() {
		return aws.Credentials{}, ErrCredentialsFileNoKeys
	}

	creds.CanExpire = true
	creds.Expires = time.Now().Add(credentialsFileTTL)

	return creds, nil
}
