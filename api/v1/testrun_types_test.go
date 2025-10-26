// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: Copyright (c) 2025 ReviewSignal
//

package v1

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestTestRunTypes(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "TestRun Types Suite")
}

var _ = Describe("TestRun", func() {
	Describe("Phase()", func() {
		It("should return Draft when state is empty", func() {
			testRun := TestRun{
				Spec: TestRunSpec{
					State: "",
				},
			}
			Expect(testRun.Phase()).To(Equal(TestRunPhaseDraft))
		})

		It("should return Draft when state is Draft", func() {
			testRun := TestRun{
				Spec: TestRunSpec{
					State: TestRunStateDraft,
				},
			}
			Expect(testRun.Phase()).To(Equal(TestRunPhaseDraft))
		})

		It("should return Archived when state is Archived", func() {
			testRun := TestRun{
				Spec: TestRunSpec{
					State: TestRunStateArchived,
				},
			}
			Expect(testRun.Phase()).To(Equal(TestRunPhaseArchived))
		})

		It("should return Failed when any worker is Failed", func() {
			testRun := TestRun{
				Spec: TestRunSpec{
					State: TestRunStateActive,
				},
				Status: TestRunStatus{
					WorkerStatuses: []TestRunWorkerStatus{
						{Name: "worker1", Phase: TestRunPhaseRunning},
						{Name: "worker2", Phase: TestRunPhaseFailed},
						{Name: "worker3", Phase: TestRunPhaseCompleted},
					},
				},
			}
			Expect(testRun.Phase()).To(Equal(TestRunPhaseFailed))
		})

		It("should return Canceled when any worker is Canceled", func() {
			testRun := TestRun{
				Spec: TestRunSpec{
					State: TestRunStateActive,
				},
				Status: TestRunStatus{
					WorkerStatuses: []TestRunWorkerStatus{
						{Name: "worker1", Phase: TestRunPhaseRunning},
						{Name: "worker2", Phase: TestRunPhaseCanceled},
						{Name: "worker3", Phase: TestRunPhaseCompleted},
					},
				},
			}
			Expect(testRun.Phase()).To(Equal(TestRunPhaseCanceled))
		})

		It("should return Canceled when state is Canceled and no workers", func() {
			testRun := TestRun{
				Spec: TestRunSpec{
					State:   TestRunStateCanceled,
					Workers: []TestRunWorkerSpec{},
				},
			}
			Expect(testRun.Phase()).To(Equal(TestRunPhaseCanceled))
		})

		It("should return Canceling when state is Canceled and fewer worker statuses than spec workers", func() {
			testRun := TestRun{
				Spec: TestRunSpec{
					State: TestRunStateCanceled,
					Workers: []TestRunWorkerSpec{
						{ObjectReference: ObjectReference{Name: "worker1"}},
						{ObjectReference: ObjectReference{Name: "worker2"}},
					},
				},
				Status: TestRunStatus{
					WorkerStatuses: []TestRunWorkerStatus{
						{Name: "worker1", Phase: TestRunPhaseQueued}, // Not canceled yet
					},
				},
			}
			Expect(testRun.Phase()).To(Equal(TestRunPhaseCanceling))
		})

		It("should return Pending when state is Active and fewer worker statuses than spec workers", func() {
			testRun := TestRun{
				Spec: TestRunSpec{
					State: TestRunStateActive,
					Workers: []TestRunWorkerSpec{
						{ObjectReference: ObjectReference{Name: "worker1"}},
						{ObjectReference: ObjectReference{Name: "worker2"}},
					},
				},
				Status: TestRunStatus{
					WorkerStatuses: []TestRunWorkerStatus{
						{Name: "worker1", Phase: TestRunPhaseQueued},
					},
				},
			}
			Expect(testRun.Phase()).To(Equal(TestRunPhasePending))
		})

		It("should return Pending when state is Active and no worker statuses", func() {
			testRun := TestRun{
				Spec: TestRunSpec{
					State: TestRunStateActive,
				},
				Status: TestRunStatus{
					WorkerStatuses: []TestRunWorkerStatus{},
				},
			}
			Expect(testRun.Phase()).To(Equal(TestRunPhasePending))
		})

		It("should return the lowest worker phase when all workers have statuses", func() {
			testRun := TestRun{
				Spec: TestRunSpec{
					State: TestRunStateActive,
					Workers: []TestRunWorkerSpec{
						{ObjectReference: ObjectReference{Name: "worker1"}},
						{ObjectReference: ObjectReference{Name: "worker2"}},
						{ObjectReference: ObjectReference{Name: "worker3"}},
					},
				},
				Status: TestRunStatus{
					WorkerStatuses: []TestRunWorkerStatus{
						{Name: "worker1", Phase: TestRunPhaseCompleted},
						{Name: "worker2", Phase: TestRunPhaseQueued},
						{Name: "worker3", Phase: TestRunPhaseRunning},
					},
				},
			}
			Expect(testRun.Phase()).To(Equal(TestRunPhaseQueued))
		})

		It("should return Draft when state is Draft and no worker statuses", func() {
			testRun := TestRun{
				Spec: TestRunSpec{
					State: TestRunStateDraft,
				},
				Status: TestRunStatus{
					WorkerStatuses: []TestRunWorkerStatus{},
				},
			}
			Expect(testRun.Phase()).To(Equal(TestRunPhaseDraft))
		})
	})

	Describe("TotalWorkers()", func() {
		It("should return 0 when no workers are defined", func() {
			testRun := TestRun{
				Spec: TestRunSpec{
					Workers: []TestRunWorkerSpec{},
				},
			}
			Expect(testRun.TotalWorkers()).To(Equal(0))
		})

		It("should return 1 for each worker with NumWorkers < 1", func() {
			testRun := TestRun{
				Spec: TestRunSpec{
					Workers: []TestRunWorkerSpec{
						{ObjectReference: ObjectReference{Name: "worker1"}, NumWorkers: 0},
						{ObjectReference: ObjectReference{Name: "worker2"}, NumWorkers: -1},
					},
				},
			}
			Expect(testRun.TotalWorkers()).To(Equal(2))
		})

		It("should sum NumWorkers across all workers", func() {
			testRun := TestRun{
				Spec: TestRunSpec{
					Workers: []TestRunWorkerSpec{
						{ObjectReference: ObjectReference{Name: "worker1"}, NumWorkers: 3},
						{ObjectReference: ObjectReference{Name: "worker2"}, NumWorkers: 2},
						{ObjectReference: ObjectReference{Name: "worker3"}, NumWorkers: 5},
					},
				},
			}
			Expect(testRun.TotalWorkers()).To(Equal(10))
		})

		It("should use 1 for workers with NumWorkers not set", func() {
			testRun := TestRun{
				Spec: TestRunSpec{
					Workers: []TestRunWorkerSpec{
						{ObjectReference: ObjectReference{Name: "worker1"}, NumWorkers: 3},
						{ObjectReference: ObjectReference{Name: "worker2"}, NumWorkers: 0},
					},
				},
			}
			Expect(testRun.TotalWorkers()).To(Equal(4))
		})
	})

	Describe("SegmentSequence()", func() {
		It("should return [0, 1] for 0 or 1 total workers", func() {
			testRun := TestRun{
				Spec: TestRunSpec{
					Workers: []TestRunWorkerSpec{},
				},
			}
			Expect(testRun.SegmentSequence()).To(Equal([]string{"0", "1"}))

			testRun = TestRun{
				Spec: TestRunSpec{
					Workers: []TestRunWorkerSpec{
						{ObjectReference: ObjectReference{Name: "worker1"}, NumWorkers: 1},
					},
				},
			}
			Expect(testRun.SegmentSequence()).To(Equal([]string{"0", "1"}))
		})

		It("should return proper sequence for 2 workers", func() {
			testRun := TestRun{
				Spec: TestRunSpec{
					Workers: []TestRunWorkerSpec{
						{ObjectReference: ObjectReference{Name: "worker1"}, NumWorkers: 2},
					},
				},
			}
			Expect(testRun.SegmentSequence()).To(Equal([]string{"0", "1/2", "1"}))
		})

		It("should return proper sequence for 4 workers", func() {
			testRun := TestRun{
				Spec: TestRunSpec{
					Workers: []TestRunWorkerSpec{
						{ObjectReference: ObjectReference{Name: "worker1"}, NumWorkers: 4},
					},
				},
			}
			Expect(testRun.SegmentSequence()).To(Equal([]string{"0", "1/4", "2/4", "3/4", "1"}))
		})

		It("should handle multiple worker specs", func() {
			testRun := TestRun{
				Spec: TestRunSpec{
					Workers: []TestRunWorkerSpec{
						{ObjectReference: ObjectReference{Name: "worker1"}, NumWorkers: 1},
						{ObjectReference: ObjectReference{Name: "worker2"}, NumWorkers: 2},
					},
				},
			}
			Expect(testRun.SegmentSequence()).To(Equal([]string{"0", "1/3", "2/3", "1"}))
		})
	})

	Describe("Segments()", func() {
		It("should return single segment [0:1] for 0 or 1 total workers", func() {
			testRun := TestRun{
				Spec: TestRunSpec{
					Workers: []TestRunWorkerSpec{},
				},
			}
			Expect(testRun.Segments()).To(Equal([]string{"0:1"}))

			testRun = TestRun{
				Spec: TestRunSpec{
					Workers: []TestRunWorkerSpec{
						{ObjectReference: ObjectReference{Name: "worker1"}, NumWorkers: 1},
					},
				},
			}
			Expect(testRun.Segments()).To(Equal([]string{"0:1"}))
		})

		It("should return proper segments for 2 workers", func() {
			testRun := TestRun{
				Spec: TestRunSpec{
					Workers: []TestRunWorkerSpec{
						{ObjectReference: ObjectReference{Name: "worker1"}, NumWorkers: 2},
					},
				},
			}
			Expect(testRun.Segments()).To(Equal([]string{"0:1/2", "1/2:1"}))
		})

		It("should return proper segments for 4 workers", func() {
			testRun := TestRun{
				Spec: TestRunSpec{
					Workers: []TestRunWorkerSpec{
						{ObjectReference: ObjectReference{Name: "worker1"}, NumWorkers: 4},
					},
				},
			}
			Expect(testRun.Segments()).To(Equal([]string{"0:1/4", "1/4:2/4", "2/4:3/4", "3/4:1"}))
		})

		It("should handle multiple worker specs", func() {
			testRun := TestRun{
				Spec: TestRunSpec{
					Workers: []TestRunWorkerSpec{
						{ObjectReference: ObjectReference{Name: "worker1"}, NumWorkers: 1},
						{ObjectReference: ObjectReference{Name: "worker2"}, NumWorkers: 2},
					},
				},
			}
			Expect(testRun.Segments()).To(Equal([]string{"0:1/3", "1/3:2/3", "2/3:1"}))
		})
	})
})
