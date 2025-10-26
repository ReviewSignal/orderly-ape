// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: Copyright (c) 2025 ReviewSignal
//

package grafana

import (
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
)

func TestGrafana(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Grafana Suite")
}

func IndexBy[K comparable](f K) func(any) string {
	return func(e any) string {
		m, ok := e.(map[K]any)
		if !ok {
			return ""
		}
		return m[f].(string)
	}
}

var _ = Describe("PrepareDashboard", func() {
	It("should load and prepare dashboard template", func() {
		testName := "my-test-run"
		testTarget := "https://example.com/test"
		bucket := "test-bucket"
		dsInfluxUID := "test-influxds-uid"
		dsVictoriaLogsUID := "test-victorialogs-uid"
		version := "v1.0.0"
		startTime := time.Now().Add(-1 * time.Hour)
		endTime := time.Now()
		tags := []string{"orderly-ape/v1.0.0", "scenario/load-test", "env/prod"}

		dashboard, err := PrepareDashboard(testName, testTarget, bucket, dsInfluxUID, dsVictoriaLogsUID, version, &startTime, &endTime, 10, tags)
		Expect(err).NotTo(HaveOccurred())
		Expect(dashboard).NotTo(BeNil())

		Expect(dashboard).To(MatchKeys(IgnoreExtras, Keys{
			"title": Equal("Test Results - my-test-run"),
			"tags":  Equal([]any{"orderly-ape/v1.0.0", "scenario/load-test", "env/prod"}),
			"templating": MatchKeys(IgnoreExtras, Keys{
				"list": MatchElements(IndexBy("name"), IgnoreExtras, Elements{
					"bucket": MatchKeys(IgnoreExtras, Keys{
						"type":        Equal("constant"),
						"skipUrlSync": Equal(true),
						"hide":        Equal(float64(2)),
						"query":       Equal("test-bucket"),
					}),
					"testid": MatchKeys(IgnoreExtras, Keys{
						"type":        Equal("constant"),
						"skipUrlSync": Equal(true),
						"hide":        Equal(float64(2)),
						"query":       Equal("my-test-run"),
					}),
					"target": MatchKeys(IgnoreExtras, Keys{
						"type":        Equal("constant"),
						"skipUrlSync": Equal(true),
						"hide":        Equal(float64(2)),
						"query":       Equal("https://example.com/test"),
					}),
					"aggregation_interval": MatchKeys(IgnoreExtras, Keys{
						"type":        Equal("constant"),
						"skipUrlSync": Equal(true),
						"hide":        Equal(float64(2)),
						"label":       Equal("Aggregation Interval"),
						"description": Equal("The interval data was aggregated before being stored"),
						"query":       Equal("10s"),
					}),
				}),
			}),
		}))
	})

	It("should handle nil times", func() {
		testName := "my-test-run"
		testTarget := "https://example.com/test"
		bucket := "test-bucket"
		dsInfluxUID := "test-influxds-uid"
		dsVictoriaLogsUID := "test-victorialogs-uid"
		version := "v1.0.0"
		tags := []string{"orderly-ape/v1.0.0"}

		dashboard, err := PrepareDashboard(testName, testTarget, bucket, dsInfluxUID, dsVictoriaLogsUID, version, nil, nil, 10, tags)
		Expect(err).NotTo(HaveOccurred())
		Expect(dashboard).NotTo(BeNil())

		// Verify title was updated
		title, ok := dashboard["title"].(string)
		Expect(ok).To(BeTrue())
		Expect(title).To(Equal("Test Results - my-test-run"))
	})

	It("should replace datasource UID in queries", func() {
		testName := "my-test-run"
		testTarget := "https://example.com/test"
		bucket := "test-bucket"
		dsInfluxUID := "custom-datasource-123"
		dsVictoriaLogsUID := "test-victorialogs-uid"
		version := "v1.0.0"
		startTime := time.Now()
		tags := []string{"orderly-ape/v1.0.0"}

		dashboard, err := PrepareDashboard(testName, testTarget, bucket, dsInfluxUID, dsVictoriaLogsUID, version, &startTime, nil, 10, tags)
		Expect(err).NotTo(HaveOccurred())
		Expect(dashboard).NotTo(BeNil())

		// Check that panels have the custom datasource UID
		panels, ok := dashboard["panels"].([]any)
		Expect(ok).To(BeTrue())
		Expect(panels).ToNot(BeEmpty())

		// Verify at least one panel has the datasource
		foundDatasource := false
		for _, panelInterface := range panels {
			if panel, ok := panelInterface.(map[string]any); ok {
				if datasource, ok := panel["datasource"].(map[string]any); ok {
					if uid, ok := datasource["uid"].(string); ok && uid == dsInfluxUID {
						foundDatasource = true
						break
					}
				}
			}
		}
		Expect(foundDatasource).To(BeTrue())
	})
})
