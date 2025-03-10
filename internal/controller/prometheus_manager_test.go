package controller

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	inferencev1alpha1 "github.com/openshift/instaslice-operator/api/v1alpha1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

var _ = Describe("Instaslice Controller", func() {
	var (
		ctx        context.Context
		fakeClient client.Client
		r          *InstasliceReconciler
		instaslice *inferencev1alpha1.Instaslice
		pod        *v1.Pod
		podUUID    string
	)

	BeforeEach(func() {
		ctx = context.Background()
		scheme := runtime.NewScheme()
		Expect(inferencev1alpha1.AddToScheme(scheme)).To(Succeed())
		Expect(v1.AddToScheme(scheme)).To(Succeed())

		fakeClient = fake.NewClientBuilder().WithScheme(scheme).Build()

		r = &InstasliceReconciler{
			Client: fakeClient,
		}

		podUUID = "test-pod-uuid"

		pod = &v1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-pod",
				Namespace: InstaSliceOperatorNamespace,
				UID:       types.UID(podUUID),
				Finalizers: []string{
					FinalizerName,
				},
			},
		}

		Expect(fakeClient.Create(ctx, pod)).To(Succeed())

		instaslice = &inferencev1alpha1.Instaslice{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-instaslice",
				Namespace: InstaSliceOperatorNamespace,
			},
			Spec: inferencev1alpha1.InstasliceSpec{
				PodAllocationRequests: map[types.UID]inferencev1alpha1.AllocationRequest{
					types.UID(podUUID): {
						Profile: "test-profile",
						PodRef: v1.ObjectReference{
							Name:      pod.Name,
							Namespace: InstaSliceOperatorNamespace,
							UID:       pod.UID,
						},
						Resources: v1.ResourceRequirements{},
					},
				},
			},
			Status: inferencev1alpha1.InstasliceStatus{
				PodAllocationResults: map[types.UID]inferencev1alpha1.AllocationResult{
					types.UID(podUUID): {
						AllocationStatus:            inferencev1alpha1.AllocationStatus{AllocationStatusController: inferencev1alpha1.AllocationStatusCreating},
						GPUUUID:                     "GPU-12345",
						Nodename:                    "fake-node",
						ConfigMapResourceIdentifier: "fake-configmap-uid",
					},
				},
			},
		}
		Expect(fakeClient.Create(ctx, instaslice)).To(Succeed())
	})

	// Test IncrementTotalProcessedGpuSliceMetrics
	It("should increment total processed GPU slice metrics", func() {
		err := r.IncrementTotalProcessedGpuSliceMetrics(ctx, *instaslice, "node-1", "gpu-1", "1g.5gb")
		Expect(err).ToNot(HaveOccurred())
	})

	// Test UpdateGpuSliceMetrics
	It("should update GPU slice metrics", func() {
		err := r.UpdateGpuSliceMetrics("node-1", "gpu-1", 3, 5)
		Expect(err).ToNot(HaveOccurred())
	})

	// Test UpdateDeployedPodTotalMetrics
	It("should update deployed pod total metrics", func() {
		err := r.UpdateDeployedPodTotalMetrics("node-1", "gpu-1", "namespace-1", "pod-1", "profile-1", 2)
		Expect(err).ToNot(HaveOccurred())
	})

	// Test UpdateCompatibleProfilesMetrics
	It("should update compatible profiles metrics", func() {
		instaslice := inferencev1alpha1.Instaslice{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "instaslice-1",
				Namespace: "default",
			},
		}
		err := r.UpdateCompatibleProfilesMetrics(instaslice, "node-1")
		Expect(err).ToNot(HaveOccurred())
	})
})
