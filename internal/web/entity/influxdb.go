// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: Copyright (c) 2025 ReviewSignal
//

package entity

import (
	"net/url"

	"github.com/a-h/templ"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	apev1 "github.com/ReviewSignal/orderly-ape/api/v1"
	"github.com/ReviewSignal/orderly-ape/internal/web/datatable"
)

type InfluxDB struct {
	*apev1.InfluxDB
}

type InfluxDBList []InfluxDB

func WrapInfluxDB(i *apev1.InfluxDB) InfluxDB {
	return InfluxDB{i}
}

func WrapInfluxDBList(items []apev1.InfluxDB) InfluxDBList {
	wrapped := make(InfluxDBList, len(items))
	for i := range items {
		wrapped[i] = WrapInfluxDB(&items[i])
	}
	return wrapped
}

func (i InfluxDB) NamespacedName() string {
	return client.ObjectKeyFromObject(i.InfluxDB).String()
}

func (i InfluxDB) ObjectLink() templ.SafeURL {
	path, _ := url.JoinPath("influxdb", i.Name)
	return templ.URL(path)
}

func (i InfluxDB) VerboseName() string {
	return "InfluxDB Connection"
}

func (i InfluxDB) EntryStatus() datatable.Status {
	if !i.GetDeletionTimestamp().IsZero() {
		return datatable.Status{
			Icon:        datatable.StatusIconDeleteing,
			Description: "InfluxDB is being deleted",
		}
	}

	condition := meta.FindStatusCondition(i.Status.Conditions, apev1.InfluxDBConditionReady)
	switch {
	case condition != nil && condition.Status == metav1.ConditionTrue:
		return datatable.Status{
			Icon:        datatable.StatusIconReady,
			Title:       "InfluxDB is ready",
			Description: condition.Message,
		}
	case condition != nil && condition.Status == metav1.ConditionFalse:
		return datatable.Status{
			Icon:        datatable.StatusIconFailed,
			Title:       "InfluxDB is not ready",
			Description: condition.Message,
		}
	default:
		return datatable.Status{
			Icon:        datatable.StatusIconUnknown,
			Description: "InfluxDB status is unknown",
		}
	}
}
