// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: Copyright (c) 2025 ReviewSignal
//

package testrunutil

import (
	"strings"

	"k8s.io/apimachinery/pkg/util/json"

	apev1 "github.com/ReviewSignal/orderly-ape/api/v1"
	"github.com/ReviewSignal/orderly-ape/internal/web/forms"
)

type Location struct {
	Name       string `json:"name"`
	NumWorkers int32  `json:"workers"`
}

func ParseLocations(data map[string]string, form *forms.Form) ([]apev1.TestRunWorkerSpec, error) {
	if data["locations"] == "" {
		return nil, nil
	}

	locations := make([]Location, 0)
	if err := json.Unmarshal([]byte(data["locations"]), &locations); err != nil {
		err := forms.FieldErrorf(err, "Invalid locations data")
		form.AddFieldError("locations", err)
		return nil, err
	}

	locationSet := make(map[string]struct{})
	workers := make([]apev1.TestRunWorkerSpec, 0, len(locations))
	for _, l := range locations {
		if l.Name == "" {
			err := forms.FieldErrorf(nil, "Location name is required")
			form.AddFieldError("locations", err)
			return nil, err
		}

		if l.NumWorkers < 1 {
			err := forms.FieldErrorf(nil, "Number of workers must be at least 1")
			form.AddFieldError("locations", err)
			return nil, err
		}

		workerParts := strings.Split(l.Name, "/")
		if len(workerParts) != 2 {
			err := forms.FieldErrorf(nil, "Invalid worker format")
			form.AddFieldError("locations", err)
			return nil, err
		}

		if _, exists := locationSet[l.Name]; exists {
			err := forms.FieldErrorf(nil, "Duplicate location: %s", workerParts[1])
			form.AddFieldError("locations", err)
			return nil, err
		}

		locationSet[l.Name] = struct{}{}
		workers = append(workers, apev1.TestRunWorkerSpec{
			ObjectReference: apev1.ObjectReference{
				Name:      workerParts[1],
				Namespace: workerParts[0],
			},
			NumWorkers: l.NumWorkers,
		})
	}
	return workers, nil
}

func ParseEnvVars(data map[string]string, form *forms.Form) ([]apev1.EnvVar, error) {
	if data["env"] == "" {
		return nil, nil
	}

	type envVar struct {
		Name  string `json:"name"`
		Value string `json:"value"`
	}
	envVars := make([]envVar, 0)
	if err := json.Unmarshal([]byte(data["env"]), &envVars); err != nil {
		err := forms.FieldErrorf(err, "Invalid environment variables data")
		form.AddFieldError("env", err)
		return nil, err
	}

	result := make([]apev1.EnvVar, 0, len(envVars))
	for _, ev := range envVars {
		if ev.Name != "" {
			result = append(result, apev1.EnvVar{
				Name:  ev.Name,
				Value: ev.Value,
			})
		}
	}
	return result, nil
}

func ParseLabels(data map[string]string, form *forms.Form) (map[string]string, error) {
	if data["labels"] == "" {
		return nil, nil
	}

	type label struct {
		Name  string `json:"name"`
		Value string `json:"value"`
	}
	labels := make([]label, 0)
	if err := json.Unmarshal([]byte(data["labels"]), &labels); err != nil {
		err := forms.FieldErrorf(err, "Invalid labels data")
		form.AddFieldError("labels", err)
		return nil, err
	}

	result := make(map[string]string)
	for _, l := range labels {
		if l.Name != "" {
			result[l.Name] = l.Value
		}
	}
	return result, nil
}

// MergeEnvVars merges environment variables, with testRunEnvVars taking priority over scenarioEnvVars
func MergeEnvVars(scenarioEnvVars, testRunEnvVars []apev1.EnvVar) []apev1.EnvVar {
	envVarMap := make(map[string]string)

	// Add scenario env vars first
	for _, ev := range scenarioEnvVars {
		envVarMap[ev.Name] = ev.Value
	}

	// Override with test run env vars
	for _, ev := range testRunEnvVars {
		envVarMap[ev.Name] = ev.Value
	}

	// Convert back to slice
	result := make([]apev1.EnvVar, 0, len(envVarMap))
	for name, value := range envVarMap {
		result = append(result, apev1.EnvVar{
			Name:  name,
			Value: value,
		})
	}

	return result
}

// MergeLabels merges labels, with testRunLabels taking priority over scenarioLabels
func MergeLabels(scenarioLabels, testRunLabels map[string]string) map[string]string {
	if scenarioLabels == nil && testRunLabels == nil {
		return nil
	}

	result := make(map[string]string)

	// Add scenario labels first
	for k, v := range scenarioLabels {
		result[k] = v
	}

	// Override with test run labels
	for k, v := range testRunLabels {
		result[k] = v
	}

	return result
}
