package cnpgi

import (
	"strconv"

	cnpgv1 "github.com/cloudnative-pg/api/pkg/api/v1"
	"github.com/cloudnative-pg/cnpg-i-machinery/pkg/pluginhelper/common"
)

// PluginName is the name of the plugin from the instance manager
// Point-of-view.
//
// TODO: yes I know, this is in the "operator" package too,
// but there's no space in this ticket to untangle the dependencies.
const PluginName = "klio.cnpg.io"

// Retention contains the retention policy configuration.
type Retention struct {
	KeepLatest  *int
	KeepAnnual  *int
	KeepMonthly *int
	KeepWeekly  *int
	KeepDaily   *int
	KeepHourly  *int
}

// IsEmpty checks if the retention configuration is empty or not.
func (r *Retention) IsEmpty() bool {
	if r == nil {
		return true
	}

	emptyRetention := Retention{}

	return *r == emptyRetention
}

// extractRetentionFromCluster reads retention policy settings.
func extractRetentionFromCluster(cluster *cnpgv1.Cluster) *Retention {
	pluginData := common.NewPlugin(*cluster, PluginName)

	conf := Retention{}
	conf.KeepLatest = tryParseInt(pluginData.Parameters["keepLatest"])
	conf.KeepAnnual = tryParseInt(pluginData.Parameters["keepAnnual"])
	conf.KeepMonthly = tryParseInt(pluginData.Parameters["keepMonthly"])
	conf.KeepWeekly = tryParseInt(pluginData.Parameters["keepWeekly"])
	conf.KeepDaily = tryParseInt(pluginData.Parameters["keepDaily"])
	conf.KeepHourly = tryParseInt(pluginData.Parameters["keepHourly"])

	return &conf
}

func tryParseInt(value string) *int {
	result, err := strconv.Atoi(value)
	if err != nil {
		return nil
	}

	return &result
}
