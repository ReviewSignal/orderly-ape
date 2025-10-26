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

type Worker struct {
	*apev1.Worker
}

type WorkerList []Worker

func WrapWorker(w *apev1.Worker) Worker {
	return Worker{w}
}

func WrapWorkerList(items []apev1.Worker) WorkerList {
	wrapped := make(WorkerList, len(items))
	for i := range items {
		wrapped[i] = WrapWorker(&items[i])
	}
	return wrapped
}

func (w Worker) NamespacedName() string {
	return client.ObjectKeyFromObject(w.Worker).String()
}

func (w Worker) ObjectLink() templ.SafeURL {
	path, _ := url.JoinPath("workers", w.Name)
	return templ.URL(path)
}

func (w Worker) VerboseName() string {
	return "Location"
}

func (w Worker) EntryStatus() datatable.Status {
	if !w.GetDeletionTimestamp().IsZero() {
		return datatable.Status{
			Icon:        datatable.StatusIconDeleteing,
			Description: "Location worker is being deleted",
		}
	}

	condition := meta.FindStatusCondition(w.Status.Conditions, apev1.WorkerConditionReady)
	switch {
	case condition != nil && condition.Status == metav1.ConditionTrue:
		return datatable.Status{
			Icon:        datatable.StatusIconReady,
			Title:       "Location worker is ready",
			Description: condition.Message,
		}
	case condition != nil && condition.Status == metav1.ConditionFalse:
		return datatable.Status{
			Icon:        datatable.StatusIconFailed,
			Title:       "Location worker is not ready",
			Description: condition.Message,
		}
	default:
		return datatable.Status{
			Icon:        datatable.StatusIconUnknown,
			Description: "Location worker status is unknown",
		}
	}
}
