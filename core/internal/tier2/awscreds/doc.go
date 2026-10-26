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

// Package awscreds contains AWS SDK plumbing for credentials and CA
// bundles that live in mounted files: the S3 access keys of the tier2
// repositories and the trust CA bundle of their endpoints are provided
// by the operator as files whose content is refreshed in place when the
// backing Secret rotates. Klio must follow those rotations without
// restarting the server process, so every consumer below re-reads its
// file when the AWS SDK asks for credentials again.
//
// The AWS SDK for Go v2 does not provide this behavior out of the box:
//
//   - The v1 SDK had a credentials.SharedCredentialsProvider that parsed
//     the shared credentials file on every Retrieve call, so a rewritten
//     file was picked up naturally. In v2 that parsing moved to
//     config-time (config.LoadSharedConfigProfile): whatever it resolves
//     for a static profile is baked into a static credentials provider
//     when the configuration is built, and no runtime provider re-reads
//     the file afterwards.
//   - Every provider handed to config.LoadDefaultConfig is wrapped in an
//     aws.CredentialsCache that only asks the underlying provider again
//     once credentials expire — and static, file-based credentials never
//     expire. The net effect is that credentials read from a shared
//     credentials file are cached for the process lifetime, long after
//     the file itself has been rewritten.
//
// The result is ExpiredToken failures whenever an external process
// rotates the file under a running client, which upstream declined to
// treat as a bug to fix:
// https://github.com/aws/aws-sdk-go-v2/issues/1449 and
// https://github.com/aws/aws-sdk-go-v2/issues/2135 (closed as not
// planned). The equivalent capability exists in the Java v2 SDK, which
// gained profile reloading on file change in
// https://github.com/aws/aws-sdk-java-v2/issues/1754 — the absence from
// the Go v2 SDK is a known gap, not an intentional design.
//
// The workaround is to make the credentials expire: the credentials file
// provider re-reads the file on every Retrieve call and stamps each
// result with a short artificial TTL (see credentialsFileTTL), so the
// wrapping CredentialsCache re-invokes it shortly after and rotation is
// followed with at most that delay.
package awscreds
