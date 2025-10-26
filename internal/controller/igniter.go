// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: Copyright (c) 2025 ReviewSignal
//

package controller

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"golang.org/x/sync/errgroup"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clientset "k8s.io/client-go/kubernetes"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	mcontext "sigs.k8s.io/multicluster-runtime/pkg/context"

	apev1 "github.com/ReviewSignal/orderly-ape/api/v1"
	k6api "github.com/ReviewSignal/orderly-ape/internal/k6/api"
)

type Igniters map[string]*Igniter

type Igniter struct {
	*errgroup.Group
	TestRun        *apev1.TestRun
	PodsNamespace  string
	PodNames       []string
	cancel         context.CancelFunc
	workerFullName string
	groupCtx       context.Context

	mu        sync.Mutex
	started   bool
	error     error
	clientset clientset.Interface
}

func (i *Igniter) Started() bool {
	i.mu.Lock()
	defer i.mu.Unlock()
	return i.started
}

func (i *Igniter) Error() error {
	i.mu.Lock()
	defer i.mu.Unlock()
	return i.error
}

// +kubebuilder:rbac:groups=core,resources=pods/proxy,verbs=get;post;put;patch;delete

func (i *Igniter) Start(ctx context.Context) error {
	l := log.FromContext(ctx, "testrun", client.ObjectKeyFromObject(i.TestRun), "worker", i.workerFullName, "startTime", i.TestRun.Status.StartTime)
	l.Info("Igniter started")
	select {
	case <-ctx.Done():
		return nil
	case <-time.After(time.Until(i.TestRun.Status.StartTime.Time)):
		i.mu.Lock()
		i.started = true
		i.mu.Unlock()
		l.Info("Starting test runs")

		for _, podName := range i.PodNames {
			pod := &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      podName,
					Namespace: i.PodsNamespace,
				},
			}

			i.Go(func() error {
				l.Info("IGNITE", "pod", pod.Name)
				client := i.clientset.CoreV1().RESTClient()
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
			i.mu.Lock()
			i.error = err
			i.mu.Unlock()
			return err
		}
	}
	return nil
}

func (i *Igniter) Stop() {
	i.cancel()
}

func (r *TestRunWorkerReconciler) createIgniter(ctx context.Context, testrun *apev1.TestRun, job *batchv1.Job) (*Igniter, error) {
	workerFullName, ok := mcontext.ClusterFrom(ctx)
	if !ok {
		return nil, fmt.Errorf("unable to get worker cluster from context")
	}
	parts := strings.SplitN(workerFullName, "/", 2)
	if len(parts) != 2 {
		return nil, fmt.Errorf("invalid worker cluster name: %s", workerFullName)
	}
	workerName := parts[1]

	cluster, err := r.GetCluster(ctx, workerFullName)
	if err != nil {
		return nil, err
	}

	cs, err := clientset.NewForConfig(cluster.GetConfig())
	if err != nil {
		return nil, err
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	igniterName := fmt.Sprintf("%s/%s", testrun.Name, workerName)
	if _, found := r.igniters[igniterName]; found {
		return r.igniters[igniterName], nil
	}

	ws := workerStatusByName(workerName, testrun.Status.WorkerStatuses)
	if ws == nil {
		return nil, fmt.Errorf("worker status not found for worker: %s", workerName)
	}

	if ws.Phase != apev1.TestRunPhaseReady || testrun.Status.StartTime == nil {
		return nil, fmt.Errorf("job is not ready to be ignited")
	}

	pods, err := r.getPods(ctx, job)
	if err != nil {
		return nil, err
	}
	podNames := make([]string, len(pods))
	for i, pod := range pods {
		podNames[i] = pod.Name
	}

	if len(ws.AssignedSegments) != len(podNames) {
		return nil, fmt.Errorf("expected %d pods, got %d: %v", len(ws.AssignedSegments), len(podNames), podNames)
	}

	l := log.FromContext(ctx).WithValues("job", testrun.Name)

	// create a new context with logger
	ctx = ctrl.LoggerInto(context.Background(), l)
	ctx, cancel := context.WithCancel(ctx)
	g, ctx := errgroup.WithContext(ctx)

	igniter := &Igniter{
		TestRun:        testrun,
		Group:          g,
		PodNames:       podNames,
		PodsNamespace:  job.Namespace,
		cancel:         cancel,
		workerFullName: workerFullName,
		groupCtx:       ctx,
		clientset:      cs,
	}
	r.igniters[igniterName] = igniter

	// Start the igniter in a goroutine. Errors are tracked in igniter.Error
	//nolint:errcheck // Errors are tracked in igniter.Error field
	go igniter.Start(ctx)

	return igniter, nil
}

func (r *TestRunWorkerReconciler) removeIgniter(testrun *apev1.TestRun, workerName string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	delete(r.igniters, fmt.Sprintf("%s/%s", testrun.Name, workerName))
}
