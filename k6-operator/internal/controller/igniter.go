//  SPDX-License-Identifier: MIT
//  SPDX-FileCopyrightText: 2024 ReviewSignal

package controller

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"golang.org/x/sync/errgroup"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/log"

	k6api "github.com/ReviewSignal/loadtesting/k6-operator/internal/k6/api"
	loadtestingapi "github.com/ReviewSignal/loadtesting/k6-operator/internal/loadtesting/api"
)

type Igniters map[string]*Igniter

type Igniter struct {
	*errgroup.Group
	Started  bool
	Error    error
	Job      *loadtestingapi.Job
	PodNames []string
	groupCtx context.Context
}

//+kubebuilder:rbac:groups=core,resources=pods/proxy,verbs=get;post;put;patch;delete

func (i *Igniter) Start(ctx context.Context, r *TestRunReconciler) error {
	select {
	case <-ctx.Done():
		return nil
	case <-time.After(time.Until(*i.Job.TestRun.StartTestAt)):
		i.Started = true
		l := log.FromContext(ctx)
		l.Info("Starting test runs")

		for _, podName := range i.PodNames {
			pod := &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      podName,
					Namespace: i.Job.GetNamespace(),
				},
			}

			i.Go(func() error {
				l.Info("IGNITE", "pod", pod.Name)
				client := r.clientset.CoreV1().RESTClient()
				status := k6api.StatusRequest{
					Data: k6api.StatusData{
						ID:   "default",
						Type: "status",
						Attributes: k6api.StatusAttributes{
							Paused: falsePtr,
						},
					},
				}
				statusObj, _ := json.Marshal(status)

				resp, err := client.Patch("application/json").
					Resource("pods").
					SubResource("proxy").
					Namespace(pod.Namespace).
					Name(pod.Name).
					Suffix("/v1/status").
					Body(statusObj).
					DoRaw(i.groupCtx)
				l.Info("STATUS UPDATE", "resp", string(resp), "body", string(statusObj))

				return err
			})
		}

		if err := i.Wait(); err != nil {
			i.Error = err
			return err
		}
	}
	return nil
}

func (r *TestRunReconciler) createIgniter(ctx context.Context, job *loadtestingapi.Job) (*Igniter, error) {
	if _, found := r.igniters[job.Name]; found {
		return r.igniters[job.Name], nil
	}

	if !job.TestRun.Ready || job.TestRun.StartTestAt == nil {
		return nil, fmt.Errorf("job '%s' is not ready to start", job.Name)
	}

	pods, err := r.getPods(ctx, job)
	if err != nil {
		return nil, err
	}
	podNames := make([]string, len(pods))
	for i, pod := range pods {
		podNames[i] = pod.Name
	}
	if len(job.AssignedSegments) != len(podNames) {
		return nil, fmt.Errorf("expected %d pods, got %d: %v", len(job.AssignedSegments), len(podNames), podNames)
	}

	l := log.FromContext(ctx).WithValues("job", job.Name)

	// create a new context with logger
	ctx = ctrl.LoggerInto(context.Background(), l)
	g, ctx := errgroup.WithContext(ctx)

	igniter := &Igniter{
		Job:      job,
		Group:    g,
		PodNames: podNames,
		groupCtx: ctx,
	}
	r.igniters[job.Name] = igniter

	go igniter.Start(ctx, r)

	return igniter, nil
}
