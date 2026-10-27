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

package retention

import (
	"errors"
	"io/fs"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// The CNPG-I plugin runs "retention set" and "retention get" before every
// backup, with the configuration the operator generates: its source section
// leaves buffer_size to the default. Neither command reads the source
// section, so both must accept it, or no source section at all, and go on
// to connect to the server. The certificates are missing here, so the
// connection is what fails.
func TestRetentionCommandsReachTheServer(t *testing.T) {
	t.Cleanup(viper.Reset)

	certificate := filepath.Join(t.TempDir(), "missing.crt")
	client := `
client:
  cluster_name: cluster-example
  wal:
    address: 127.0.0.1:1
    server_cert_path: ` + certificate + `
    client_cert_path: ` + certificate + `
    client_key_path: ` + certificate + `
tier1_retention:
  latest: 3
`
	configurations := map[string]string{
		"operator source section": `
source:
  dsn: user=postgres replication=yes application_name=klio
  standard_dsn: user=postgres application_name=klio
  slot: klio
  buffer_size: 0
` + client,
		"no source section": client,
	}

	for name, configuration := range configurations {
		for _, cmd := range []*cobra.Command{getCmd, setCmd} {
			t.Run(cmd.Name()+" with "+name, func(t *testing.T) {
				viper.Reset()
				viper.SetConfigType("yaml")
				if err := viper.ReadConfig(strings.NewReader(configuration)); err != nil {
					t.Fatalf("viper.ReadConfig() returned an unexpected error: %v", err)
				}

				if err := cmd.RunE(cmd, nil); !errors.Is(err, fs.ErrNotExist) {
					t.Errorf("RunE() = %v, want the missing server certificate error", err)
				}
			})
		}
	}
}
