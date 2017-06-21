package collector

// Collects metrics produced by marathon. Simply pulls /metrics from the marathon
//  leader and sends all well-formated metrics

import (
	"fmt"
	"fullerite/config"
	"fullerite/dropwizard"
	"fullerite/metric"
	"fullerite/util"
	"net/http"
	"time"

	l "github.com/Sirupsen/logrus"
)

var (
	sendMarathonMetrics = (*MarathonStats).sendMarathonMetrics
	getMarathonMetrics  = (*MarathonStats).getMarathonMetrics

	getMarathonMetricsURL = func(host string) string { return fmt.Sprintf("http://%s/metrics", host) }
)

const (
	marathonGetTimeout = 10 * time.Second
)

// MarathonStats Collector for marathon leader stats
type MarathonStats struct {
	baseCollector
	client        http.Client
	marathonHost  string
	paastaCluster string
}

func init() {
	RegisterCollector("MarathonStats", newMarathonStats)
}

func newMarathonStats(channel chan metric.Metric, initialInterval int, log *l.Entry) Collector {
	m := new(MarathonStats)

	m.log = log
	m.channel = channel
	m.interval = initialInterval
	m.name = "MarathonStats"
	m.client = http.Client{Timeout: marathonGetTimeout}

	return m
}

// Configure just calls the default configure
func (m *MarathonStats) Configure(configMap map[string]interface{}) {
	m.configureCommonParams(configMap)

	c := config.GetAsMap(configMap)
	if marathonHost, exists := c["marathonHost"]; exists && len(marathonHost) > 0 {
		m.marathonHost = marathonHost
	} else {
		m.log.Error("Marathon host not specified in config")
	}

	if paastaCluster, exists := c["paastaCluster"]; exists && len(paastaCluster) > 0 {
		m.paastaCluster = paastaCluster
	} else {
		// If paastaCluster is not specified, don't use it
		m.paastaCluster = ""
	}
}

// Collect compares the leader against this hosts's hostaname and sends metrics if this is the leader
func (m *MarathonStats) Collect() {
	// Non-marathon-leaders forward requests to the leader, so only the leader's metrics matter
	if leader, err := util.IsLeader(m.marathonHost, "v2/leader", m.client); leader && err == nil {
		go sendMarathonMetrics(m)
	} else if err != nil {
		m.log.Error("Error finding leader: ", err)
	} else {
		m.log.Debug("Not the leader, not sending metrics")
	}
}

func (m *MarathonStats) sendMarathonMetrics() {
	metrics := getMarathonMetrics(m)
	for _, metric := range metrics {
		m.Channel() <- metric
	}
}

func (m *MarathonStats) getMarathonMetrics() []metric.Metric {
	url := getMarathonMetricsURL(m.marathonHost)

	contents, err := util.MarathonGet(url, m.client)
	if err != nil {
		m.log.Error("Could not load metrics from marathon: ", err.Error())
		return nil
	}

	metrics, err := dropwizard.Parse(contents, "java-1.1", true)

	if err != nil {
		m.log.Error("Unable to decode marathon metrics JSON: ", err)
		return nil
	}

	metric.AddToAll(&metrics, map[string]string{
		"service": "marathon",
	})

	if m.paastaCluster != "" {
		metric.AddToAll(&metrics, map[string]string{
			"paasta_cluster": m.paastaCluster,
		})
	}

	return metrics
}
