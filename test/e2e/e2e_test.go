/*
Copyright 2024.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package e2e

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/openshift/instaslice-operator/test/utils"
)

//TODO: add more test cases -
// 1. delete instaslice object, fill the object with dangling slices ie no capacity available and
// verify that allocation should not exists in instaslice object.
// 2. check size and index value based on different mig slice profiles requested.
// 3. submit 3 pods with 3g.20gb slice and verify that two allocations exists in instaslice object.
// 4. submit a test pod with 1g.20gb slice and later delete it. verify the allocation status to be
// in state deleting

var _ = Describe("controller", Ordered, func() {
	var namespace string = "instaslice-operator-system"
	var defaultNamespace string = "default"

	BeforeAll(func() {
		fmt.Println("Setting up Kind cluster")
		cmd := exec.Command("kind", "create", "cluster", "--image", "kindest/node:v1.30.0@sha256:047357ac0cfea04663786a612ba1eaba9702bef25227a794b52890dd8bcd692e")
		output, err := cmd.CombinedOutput()
		Expect(err).NotTo(HaveOccurred(), fmt.Sprintf("Failed to create Kind cluster: %s", output))

		By("creating manager namespace")
		cmdNamespace := exec.Command("kubectl", "create", "ns", namespace)
		outputNs, err := cmdNamespace.CombinedOutput()
		Expect(err).NotTo(HaveOccurred(), fmt.Sprintf("Failed to create namespace: %s", outputNs))

		By("installing cert manager")
		cmdCm := exec.Command("kubectl", "apply", "-f", "https://github.com/cert-manager/cert-manager/releases/download/v1.15.3/cert-manager.yaml")
		outputCm, err := cmdCm.CombinedOutput()
		Expect(err).NotTo(HaveOccurred(), fmt.Sprintf("Failed to deploy cert manager: %s", outputCm))

		By("validating the cert-manager-webhook pod to be ready as expected")
		EventuallyWithOffset(1, isResourceReady, 2*time.Minute, time.Second).WithArguments("pod", "app=webhook", "cert-manager").Should(BeTrue())
	})

	AfterAll(func() {
		fmt.Println("Deleting the cluster")
		cmd := exec.Command("kind", "delete", "cluster")
		output, err := cmd.CombinedOutput()
		Expect(err).NotTo(HaveOccurred(), fmt.Sprintf("Failed to create Kind cluster: %s", output))
	})

	Context("Operator", func() {
		It("should run successfully", func() {
			var err error

			var controllerIMG = "quay.io/amalvank/instaslicev2-controller:latest"

			var daemonsetIMG = "quay.io/amalvank/instaslicev2-daemonset:latest"

			By("building the Operator images")
			cmd := exec.Command(
				"make",
				"docker-build",
				fmt.Sprintf("IMG=%s", controllerIMG),
				fmt.Sprintf("IMG_DMST=%s", daemonsetIMG),
			)
			_, err = utils.Run(cmd)
			ExpectWithOffset(1, err).NotTo(HaveOccurred())

			By("loading the manager(Operator) image on Kind")
			err = utils.LoadImageToKindClusterWithName(controllerIMG)
			ExpectWithOffset(1, err).NotTo(HaveOccurred())

			By("loading the daemonSet(Operator) image on Kind")
			err = utils.LoadImageToKindClusterWithName(daemonsetIMG)
			ExpectWithOffset(1, err).NotTo(HaveOccurred())

			By("installing CRDs")
			cmd = exec.Command("make", "install")
			_, err = utils.Run(cmd)
			ExpectWithOffset(1, err).NotTo(HaveOccurred())

			By("installing fake GPU capacity")
			cmdCm := exec.Command("kubectl", "apply", "-f", "test/e2e/resources/instaslice-fake-capacity.yaml")
			outputCm, err := cmdCm.CombinedOutput()
			Expect(err).NotTo(HaveOccurred(), fmt.Sprintf("Failed to add fake capacity to the cluster: %s", outputCm))

			By("deploying the controller-manager & controller-daemonset")
			cmd = exec.Command(
				"make",
				"deploy-emulated",
				fmt.Sprintf("IMG=%s", controllerIMG),
				fmt.Sprintf("IMG_DMST=%s", daemonsetIMG),
			)
			_, err = utils.Run(cmd)
			ExpectWithOffset(1, err).NotTo(HaveOccurred())

			By("validating that the controller-manager pod is running as expected")
			EventuallyWithOffset(1, isResourceReady, 2*time.Minute, time.Second).WithArguments("pod", "control-plane=controller-manager", namespace).Should(BeTrue())
			// We don't have a reliable source to depend on to verify if the service is up and ready
			// Issue Ref: https://github.com/kubernetes/kubernetes/issues/80828
			// (TODO) Wait for the instaslice-operator-webhook-service to be available and Ready
			/*
				By("verifying that the instaslice-operator-webhook service to be ready as expected")
				EventuallyWithOffset(1, isResourceReady, 2*time.Minute, time.Second).WithArguments("service", "app.kubernetes.io/component=webhook", namespace).Should(BeTrue())
			*/
			// Until then, adding a sleep of 1 Minute should mitigate the intermittent failures running the e2e tests.

			By("validating that the controller-daemonset pod is running as expected")
			EventuallyWithOffset(1, isResourceReady, 2*time.Minute, time.Second).WithArguments("pod", "app=controller-daemonset", namespace).Should(BeTrue())

			time.Sleep(time.Minute)

		})

		It("should apply the YAML and check if that instaslice resource exists", func() {

			cmd := exec.Command("kubectl", "apply", "-f", "test/e2e/resources/instaslice-fake-capacity.yaml")
			output, err := cmd.CombinedOutput()
			Expect(err).NotTo(HaveOccurred(), fmt.Sprintf("Failed to apply YAML: %s", output))

			checkCmd := exec.Command("kubectl", "describe", "instaslice", "-n", defaultNamespace)
			output, err = checkCmd.CombinedOutput()
			Expect(err).NotTo(HaveOccurred(), fmt.Sprintf("Resource not found: %s", output))
		})

		It("should apply the pod YAML with no requests and check if finalizer exists", func() {
			cmdPod := exec.Command("kubectl", "apply", "-f", "test/e2e/resources/test-finalizer.yaml")
			outputPod, err := cmdPod.CombinedOutput()
			Expect(err).NotTo(HaveOccurred(), fmt.Sprintf("Failed to apply YAML: %s", outputPod))

			cmdGetPod := exec.Command("kubectl", "get", "pod", "vectoradd-finalizer", "-o", "json")
			outputGetPod, err := cmdGetPod.CombinedOutput()
			Expect(err).NotTo(HaveOccurred(), fmt.Sprintf("Failed to get pod: %s", outputGetPod))

			var podObj map[string]interface{}
			err = json.Unmarshal(outputGetPod, &podObj)
			Expect(err).NotTo(HaveOccurred(), "Failed to parse pod JSON")

			finalizers, found := podObj["metadata"].(map[string]interface{})["finalizers"].([]interface{})
			Expect(found).To(BeTrue(), "Pod does not have finalizers")
			Expect(finalizers).To(ContainElement("org.instaslice/accelerator"), "Finalizer org.instaslice/accelerator not found on pod")
		})

		It("should apply the pod YAML with no requests and check the allocation in instaslice object", func() {
			cmdPod := exec.Command("kubectl", "apply", "-f", "test/e2e/resources/test-pod-no-requests.yaml")
			outputPod, err := cmdPod.CombinedOutput()
			Expect(err).NotTo(HaveOccurred(), fmt.Sprintf("Failed to apply YAML: %s", outputPod))

			cmd := exec.Command("kubectl", "get", "instaslice", "-n", defaultNamespace, "-o", "json")
			output, err := cmd.CombinedOutput()
			Expect(err).NotTo(HaveOccurred(), "Failed to get Instaslice object: "+string(output))

			// Parse the JSON output
			var result struct {
				Items []map[string]interface{} `json:"items"`
			}
			err = json.Unmarshal(output, &result)
			Expect(err).NotTo(HaveOccurred(), "Failed to parse JSON output")

			// Assume we want to check the first Instaslice object if it exists
			if len(result.Items) > 0 {
				instaslice := result.Items[0]
				spec, found := instaslice["spec"].(map[string]interface{})
				Expect(found).To(BeTrue(), "Spec not found in Instaslice object")
				allocations, found := spec["allocations"].(map[string]interface{})
				Expect(found).To(BeTrue(), "Spec.Allocations not found in Instaslice object")
				for _, data := range allocations {
					if allocation, ok := data.(map[string]interface{}); ok {
						if _, ok := allocation["allocationStatus"].(string); ok {
							Expect(ok).To(BeTrue(), "allocationStatus not found in Instaslice object")
							notCreated := findPodName(allocations, "vectoradd-no-req")
							Expect(notCreated).To(BeFalse(), "Spec.Allocations found in Instaslice object")
						}
					}
				}

			} else {
				Fail("No Instaslice objects found")
			}
		})

		It("should apply the pod YAML with small requests and check the allocation in instaslice object", func() {
			cmdPod := exec.Command("kubectl", "apply", "-f", "test/e2e/resources/test-pod-with-small-requests.yaml")
			outputPod, err := cmdPod.CombinedOutput()
			Expect(err).NotTo(HaveOccurred(), fmt.Sprintf("Failed to apply YAML: %s", outputPod))

			cmd := exec.Command("kubectl", "get", "instaslice", "-n", defaultNamespace, "-o", "json")
			output, err := cmd.CombinedOutput()
			Expect(err).NotTo(HaveOccurred(), "Failed to get Instaslice object: "+string(output))

			// Parse the JSON output
			var result struct {
				Items []map[string]interface{} `json:"items"`
			}
			err = json.Unmarshal(output, &result)
			Expect(err).NotTo(HaveOccurred(), "Failed to parse JSON output")

			// Assume we want to check the first Instaslice object if it exists
			if len(result.Items) > 0 {
				instaslice := result.Items[0]
				spec, found := instaslice["spec"].(map[string]interface{})
				Expect(found).To(BeTrue(), "Spec not found in Instaslice object")
				allocations, found := spec["allocations"].(map[string]interface{})
				Expect(found).To(BeTrue(), "Spec.Allocations not found in Instaslice object")
				for _, data := range allocations {
					if allocation, ok := data.(map[string]interface{}); ok {
						if _, ok := allocation["allocationStatus"].(string); ok {
							Expect(ok).To(BeTrue(), "allocationStatus not found in Instaslice object")
							notCreated := findPodName(allocations, "vectoradd-small-req")
							Expect(notCreated).To(BeFalse(), "Spec.Allocations found in Instaslice object")
						}
					}
				}

			} else {
				Fail("No Instaslice objects found")
			}
		})

		It("should apply the pod YAML with pod large memory requests and check the allocation in instaslice object", func() {
			cmdPod := exec.Command("kubectl", "apply", "-f", "test/e2e/resources/test-pod-with-large-memory-requests.yaml")
			outputPod, err := cmdPod.CombinedOutput()
			Expect(err).NotTo(HaveOccurred(), fmt.Sprintf("Failed to apply YAML: %s", outputPod))

			cmd := exec.Command("kubectl", "get", "instaslice", "-n", defaultNamespace, "-o", "json")
			output, err := cmd.CombinedOutput()
			Expect(err).NotTo(HaveOccurred(), "Failed to get Instaslice object: "+string(output))

			// Parse the JSON output
			var result struct {
				Items []map[string]interface{} `json:"items"`
			}
			err = json.Unmarshal(output, &result)
			Expect(err).NotTo(HaveOccurred(), "Failed to parse JSON output")

			// Assume we want to check the first Instaslice object if it exists
			if len(result.Items) > 0 {
				instaslice := result.Items[0]
				spec, found := instaslice["spec"].(map[string]interface{})
				Expect(found).To(BeTrue(), "Spec not found in Instaslice object")
				allocations, _ := spec["allocations"].(map[string]interface{})
				notCreated := findPodName(allocations, "cuda-vectoradd-large-memory")
				Expect(notCreated).To(BeTrue(), "Spec.Allocations found in Instaslice object")

			} else {
				Fail("No Instaslice objects found")
			}
		})

		It("should apply the pod YAML with pod large cpu requests and check the allocation in instaslice object", func() {
			cmdPod := exec.Command("kubectl", "apply", "-f", "test/e2e/resources/test-pod-with-large-cpu-requests.yaml")
			outputPod, err := cmdPod.CombinedOutput()
			Expect(err).NotTo(HaveOccurred(), fmt.Sprintf("Failed to apply YAML: %s", outputPod))

			cmd := exec.Command("kubectl", "get", "instaslice", "-n", defaultNamespace, "-o", "json")
			output, err := cmd.CombinedOutput()
			Expect(err).NotTo(HaveOccurred(), "Failed to get Instaslice object: "+string(output))

			// Parse the JSON output
			var result struct {
				Items []map[string]interface{} `json:"items"`
			}
			err = json.Unmarshal(output, &result)
			Expect(err).NotTo(HaveOccurred(), "Failed to parse JSON output")

			// Assume we want to check the first Instaslice object if it exists
			if len(result.Items) > 0 {
				instaslice := result.Items[0]
				spec, found := instaslice["spec"].(map[string]interface{})
				Expect(found).To(BeTrue(), "Spec not found in Instaslice object")
				allocations, _ := spec["allocations"].(map[string]interface{})
				notCreated := findPodName(allocations, "cuda-vectoradd-large-cpu")
				Expect(notCreated).To(BeTrue(), "Spec.Allocations found in Instaslice object")

			} else {
				Fail("No Instaslice objects found")
			}
		})

		It("should apply the deployment YAML and check if pod exists", func() {
			ctx := context.TODO()
			//deploymentName := "sleep-deployment"
			labelSelector := "app=sleep-app"
			cmdPod := exec.Command("kubectl", "apply", "-f", "test/e2e/resources/test-sleep-deployment.yaml")
			outputPod, err := cmdPod.CombinedOutput()
			Expect(err).NotTo(HaveOccurred(), fmt.Sprintf("Failed to apply YAML: %s", outputPod))

			cmd := exec.Command("kubectl", "get", "instaslice", "-n", defaultNamespace, "-o", "json")
			output, err := cmd.CombinedOutput()
			Expect(err).NotTo(HaveOccurred(), "Failed to get Instaslice object: "+string(output))

			// Parse the JSON output
			var result struct {
				Items []map[string]interface{} `json:"items"`
			}
			err = json.Unmarshal(output, &result)
			Expect(err).NotTo(HaveOccurred(), "Failed to parse JSON output")

			// Assume we want to check the first Instaslice object if it exists
			if len(result.Items) > 0 {
				instaslice := result.Items[0]
				_, found := instaslice["spec"].(map[string]interface{})
				Expect(found).To(BeTrue(), "Spec not found in Instaslice object")

				Eventually(func() bool {
					cmd := exec.CommandContext(ctx, "kubectl", "get", "pods", "-n", namespace, "-l", labelSelector, "--no-headers")
					output, err := cmd.CombinedOutput()
					if err != nil {
						fmt.Printf("Failed to execute kubectl: %v\n", err)
						return false
					}

					outputStr := string(output)
					podLines := strings.Split(strings.TrimSpace(outputStr), "\n")
					return len(podLines) > 0 && podLines[0] != ""
				}, 2*time.Minute, 5*time.Second).Should(BeTrue(), "No pods spawned by the deployment")

			} else {
				Fail("No Instaslice objects found")
			}
		})

		It("should apply the statefulset YAML and check if pod exists", func() {
			ctx := context.TODO()
			//statefulSetName := "sleep-statefulset"
			labelSelector := "app=sleep-statefulset"
			cmdPod := exec.Command("kubectl", "apply", "-f", "test/e2e/resources/test-sleep-statefulset.yaml")
			outputPod, err := cmdPod.CombinedOutput()
			Expect(err).NotTo(HaveOccurred(), fmt.Sprintf("Failed to apply YAML: %s", outputPod))

			cmd := exec.Command("kubectl", "get", "instaslice", "-n", defaultNamespace, "-o", "json")
			output, err := cmd.CombinedOutput()
			Expect(err).NotTo(HaveOccurred(), "Failed to get Instaslice object: "+string(output))

			// Parse the JSON output
			var result struct {
				Items []map[string]interface{} `json:"items"`
			}
			err = json.Unmarshal(output, &result)
			Expect(err).NotTo(HaveOccurred(), "Failed to parse JSON output")

			// Assume we want to check the first Instaslice object if it exists
			if len(result.Items) > 0 {
				instaslice := result.Items[0]
				_, found := instaslice["spec"].(map[string]interface{})
				Expect(found).To(BeTrue(), "Spec not found in Instaslice object")

				Eventually(func() bool {
					cmd := exec.CommandContext(ctx, "kubectl", "get", "pods", "-n", namespace, "-l", labelSelector, "--no-headers")
					output, err := cmd.CombinedOutput()
					if err != nil {
						fmt.Printf("Failed to execute kubectl: %v\n", err)
						return false
					}

					outputStr := string(output)
					podLines := strings.Split(strings.TrimSpace(outputStr), "\n")
					return len(podLines) > 0 && podLines[0] != ""
				}, 2*time.Minute, 5*time.Second).Should(BeTrue(), "No pods spawned by the statefulset")

			} else {
				Fail("No Instaslice objects found")
			}
		})

		It("should apply the job YAML and check if pod exists", func() {
			ctx := context.TODO()
			labelSelector := "app=sleep-job"
			cmdPod := exec.Command("kubectl", "apply", "-f", "test/e2e/resources/test-sleep-job.yaml")
			outputPod, err := cmdPod.CombinedOutput()
			Expect(err).NotTo(HaveOccurred(), fmt.Sprintf("Failed to apply YAML: %s", outputPod))

			cmd := exec.Command("kubectl", "get", "instaslice", "-n", defaultNamespace, "-o", "json")
			output, err := cmd.CombinedOutput()
			Expect(err).NotTo(HaveOccurred(), "Failed to get Instaslice object: "+string(output))

			// Parse the JSON output
			var result struct {
				Items []map[string]interface{} `json:"items"`
			}
			err = json.Unmarshal(output, &result)
			Expect(err).NotTo(HaveOccurred(), "Failed to parse JSON output")

			// Assume we want to check the first Instaslice object if it exists
			if len(result.Items) > 0 {
				instaslice := result.Items[0]
				_, found := instaslice["spec"].(map[string]interface{})
				Expect(found).To(BeTrue(), "Spec not found in Instaslice object")

				Eventually(func() bool {
					cmd := exec.CommandContext(ctx, "kubectl", "get", "pods", "-n", namespace, "-l", labelSelector, "--no-headers")
					output, err := cmd.CombinedOutput()
					if err != nil {
						fmt.Printf("Failed to execute kubectl: %v\n", err)
						return false
					}

					outputStr := string(output)
					podLines := strings.Split(strings.TrimSpace(outputStr), "\n")
					return len(podLines) > 0 && podLines[0] != ""
				}, 2*time.Minute, 5*time.Second).Should(BeTrue(), "No pods spawned by the job")

			} else {
				Fail("No Instaslice objects found")
			}
		})

		It("should verify nvidia.com/mig-1g.5gb is zero before submitting pods, sync with running pods, and set to zero after completion", func() {
			ctx := context.TODO()
			namespace := "default"
			nodeName := "kind-control-plane"

			resetMIGResource := func() error {
				patch := `{"status": {"capacity": {"nvidia.com/mig-1g.5gb": "0"}}}`
				cmd := exec.CommandContext(ctx, "kubectl", "patch", "node", nodeName, "--subresource=status", "--type=merge", "-p", patch)
				output, err := cmd.CombinedOutput()
				if err != nil {
					fmt.Printf("Failed to reset nvidia.com/mig-1g.5gb: %v\n", err)
					fmt.Printf("kubectl output: %s\n", string(output))
					return err
				}
				return nil
			}

			deleteInstasliceResources := func() error {
				cmd := exec.CommandContext(ctx, "kubectl", "get", "node", nodeName, "--subresource=status", "-o", "json")
				output, err := cmd.CombinedOutput()
				if err != nil {
					fmt.Printf("Failed to get node info: %v\n", err)
					return err
				}

				var nodeResult struct {
					Status struct {
						Capacity map[string]string `json:"capacity"`
					} `json:"status"`
				}
				err = json.Unmarshal(output, &nodeResult)
				if err != nil {
					fmt.Printf("Failed to parse node JSON: %v\n", err)
					return err
				}

				patchData := map[string]interface{}{
					"status": map[string]interface{}{
						"capacity": map[string]interface{}{},
					},
				}

				for resource := range nodeResult.Status.Capacity {
					if strings.HasPrefix(resource, "org.instaslice/") {
						patchData["status"].(map[string]interface{})["capacity"].(map[string]interface{})[resource] = nil
					}
				}

				patchBytes, err := json.Marshal(patchData)
				if err != nil {
					fmt.Printf("Failed to create patch data: %v\n", err)
					return err
				}

				cmdPatch := exec.CommandContext(ctx, "kubectl", "patch", "node", nodeName, "--subresource=status", "--type=merge", "-p", string(patchBytes))
				outputPatch, err := cmdPatch.CombinedOutput()
				if err != nil {
					fmt.Printf("Failed to delete org.instaslice resources: %v\n", err)
					fmt.Printf("kubectl output: %s\n", string(outputPatch))
					return err
				}

				return nil
			}

			err := resetMIGResource()
			Expect(err).NotTo(HaveOccurred(), "Failed to reset nvidia.com/mig-1g.5gb to zero")

			err = deleteInstasliceResources()
			Expect(err).NotTo(HaveOccurred(), "Failed to delete org.instaslice/* resources")

			checkMIG := func(expectedMIG string) bool {
				cmd := exec.CommandContext(ctx, "kubectl", "get", "node", "kind-control-plane", "-o", "json")
				output, err := cmd.CombinedOutput()
				if err != nil {
					fmt.Printf("Failed to execute kubectl: %v\n", err)
					fmt.Printf("kubectl output: %s\n", string(output))
					return false
				}

				var nodeResult struct {
					Status struct {
						Capacity map[string]string `json:"capacity"`
					} `json:"status"`
				}
				err = json.Unmarshal(output, &nodeResult)
				if err != nil {
					fmt.Printf("Failed to parse node capacity JSON: %v\n", err)
					return false
				}

				migCapacity, found := nodeResult.Status.Capacity["nvidia.com/mig-1g.5gb"]
				return found && migCapacity == expectedMIG
			}

			Expect(checkMIG("0")).To(BeTrue(), "nvidia.com/mig-1g.5gb is not zero before submitting pods")

			cmdApply := exec.Command("kubectl", "apply", "-f", "test/e2e/resources/capacity_test_multiple_pods.yaml")
			outputApply, err := cmdApply.CombinedOutput()
			if err != nil {
				fmt.Printf("kubectl apply error: %s\n", string(outputApply))
			}
			Expect(err).NotTo(HaveOccurred(), fmt.Sprintf("Failed to apply YAML: %s", outputApply))

			Eventually(func() bool {
				cmd := exec.CommandContext(ctx, "kubectl", "get", "pods", "-n", namespace, "--no-headers")
				output, err := cmd.CombinedOutput()
				if err != nil {
					fmt.Printf("Failed to execute kubectl: %v\n", err)
					fmt.Printf("kubectl output: %s\n", string(output))
					return false
				}

				outputStr := strings.TrimSpace(string(output))
				podLines := strings.Split(outputStr, "\n")

				for _, podLine := range podLines {
					if strings.Contains(podLine, "Running") {
						continue
					}
					return false
				}

				return true
			}, 2*time.Minute, 5*time.Second).Should(BeTrue(), "Not all pods are Running")

			runningPodsCount := 7
			Expect(checkMIG(fmt.Sprintf("%d", runningPodsCount))).To(BeTrue(), "nvidia.com/mig-1g.5gb is not in sync with the number of running pods")

			Eventually(func() bool {
				cmd := exec.CommandContext(ctx, "kubectl", "get", "pods", "-n", namespace, "--no-headers")
				output, err := cmd.CombinedOutput()
				if err != nil {
					fmt.Printf("Failed to execute kubectl: %v\n", err)
					fmt.Printf("kubectl output: %s\n", string(output))
					return false
				}

				outputStr := strings.TrimSpace(string(output))
				podLines := strings.Split(outputStr, "\n")

				for _, podLine := range podLines {
					if !strings.Contains(podLine, "Completed") {
						return false
					}
				}

				return checkMIG("0")
			}, 4*time.Minute, 5*time.Second).Should(BeTrue(), "nvidia.com/mig-1g.5gb was not set to zero after all pods completed")
		})
		It("should verify that the Kubernetes node has the specified resource and matches total GPU memory", func() {
			instasliceQuotaResourceName := "org.instaslice/accelerator-memory-quota"
			// Step 1: Get the total GPU memory from the Instaslice object
			By("Getting the Instaslice object")
			cmd := exec.Command("kubectl", "get", "instaslice", "-n", "default", "-o", "json")
			output, err := cmd.CombinedOutput()
			Expect(err).NotTo(HaveOccurred(), "Failed to get Instaslice object: "+string(output))

			// Parse the JSON output of Instaslice object
			var instasliceResult struct {
				Items []map[string]interface{} `json:"items"`
			}
			err = json.Unmarshal(output, &instasliceResult)
			Expect(err).NotTo(HaveOccurred(), "Failed to parse Instaslice JSON output")

			// Check if there are any Instaslice objects
			Expect(len(instasliceResult.Items)).To(BeNumerically(">", 0), "No Instaslice objects found")

			instaslice := instasliceResult.Items[0]
			spec, found := instaslice["spec"].(map[string]interface{})
			Expect(found).To(BeTrue(), "Spec not found in Instaslice object")

			migGPUs, found := spec["MigGPUUUID"].(map[string]interface{})
			Expect(found).To(BeTrue(), "MigGPUUUID not found in Instaslice object")

			// Calculate the total GPU memory
			totalMemoryGB := 0
			re := regexp.MustCompile(`(\d+)(GB)`)
			for _, gpuInfo := range migGPUs {
				gpuInfoStr, ok := gpuInfo.(string)
				if !ok {
					continue
				}
				matches := re.FindStringSubmatch(gpuInfoStr)
				if len(matches) == 3 {
					memoryGB, err := strconv.Atoi(matches[1])
					if err != nil {
						Fail("unable to parse GPU memory value: " + err.Error())
					}
					totalMemoryGB += memoryGB
				}
			}

			// Step 2: Get the patched resource from the node
			By(fmt.Sprintf("Verifying that node has custom resource %s", instasliceQuotaResourceName))
			cmd = exec.Command("kubectl", "get", "node", "-o", "json")
			output, err = cmd.CombinedOutput()
			Expect(err).NotTo(HaveOccurred(), "Failed to get node details: "+string(output))

			// Parse the JSON output of nodes
			var nodeResult struct {
				Items []map[string]interface{} `json:"items"`
			}
			err = json.Unmarshal(output, &nodeResult)
			Expect(err).NotTo(HaveOccurred(), "Failed to parse node JSON output")

			// Check if there are any nodes
			Expect(len(nodeResult.Items)).To(BeNumerically(">", 0), "No nodes found")

			node := nodeResult.Items[0]
			status, found := node["status"].(map[string]interface{})
			Expect(found).To(BeTrue(), "Status not found in Node object")

			capacity, found := status["capacity"].(map[string]interface{})
			Expect(found).To(BeTrue(), "Capacity resources not found in Node object")

			acceleratorMemory, found := capacity[instasliceQuotaResourceName].(string)
			Expect(found).To(BeTrue(), fmt.Sprintf("%s not found in Node object", instasliceQuotaResourceName))

			// Extract the memory size from the acceleratorMemory
			reMemory := regexp.MustCompile(`(\d+)Gi`)
			matches := reMemory.FindStringSubmatch(acceleratorMemory)
			var nodeMemoryGB int
			if len(matches) == 2 {
				nodeMemoryGB, err = strconv.Atoi(matches[1])
				Expect(err).NotTo(HaveOccurred(), "Failed to extract memory")
			} else {
				Expect(matches).To(HaveLen(2), "Failed to match memory size")
			}

			// Step 3: Verify the node's accelerator memory matches the total GPU memory in Instaslice
			By("Verifying that node's accelerator memory quota matches Instaslice total GPU memory")
			Expect(nodeMemoryGB).To(BeNumerically("==", totalMemoryGB), fmt.Sprintf("%s on node does not match total GPU memory in Instaslice object", instasliceQuotaResourceName))
		})
		// NOTE: Keep this as the last test in e2e test suite, when all workloads are deleted
		// there should be no allocations in InstaSlice object.
		It("should verify that there are no allocations on the Instaslice object", func() {
			Eventually(func() error {
				cmd := exec.Command("kubectl", "get", "instaslice", "-n", "default", "-o", "json")
				output, err := cmd.CombinedOutput()
				if err != nil {
					return fmt.Errorf("failed to get Instaslice object: %s", string(output))
				}

				var instasliceResult struct {
					Items []map[string]interface{} `json:"items"`
				}

				err = json.Unmarshal(output, &instasliceResult)
				if err != nil {
					return fmt.Errorf("failed to parse Instaslice JSON output: %s", err)
				}

				if len(instasliceResult.Items) == 0 {
					return fmt.Errorf("no Instaslice objects found")
				}

				instaslice := instasliceResult.Items[0]

				_, found := instaslice["allocations"].([]interface{})
				if found {
					return fmt.Errorf("allocations field found in Instaslice object")
				}

				return nil
			}, "60s", "5s").Should(Succeed(), "Expected no allocations in the Instaslice object")
		})

	})

	AfterEach(func() {
		time.Sleep(10 * time.Second)
		namespace = "default"
		cmdDeleteJob := exec.Command("kubectl", "delete", "job", "--all", "-n", namespace)
		outputDeleteJob, err := cmdDeleteJob.CombinedOutput()
		if err != nil {
			fmt.Printf("Failed to delete job: %s\n", string(outputDeleteJob))
		}

		cmdDeleteDeployments := exec.Command("kubectl", "delete", "deployments", "--all", "-n", namespace)
		outputDeleteDeployments, err := cmdDeleteDeployments.CombinedOutput()
		if err != nil {
			fmt.Printf("Failed to delete pods: %s\n", string(outputDeleteDeployments))
		}

		cmdDeleteStatefulsets := exec.Command("kubectl", "delete", "statefulsets", "--all", "-n", namespace)
		outputDeleteStatefulsets, err := cmdDeleteStatefulsets.CombinedOutput()
		if err != nil {
			fmt.Printf("Failed to delete pods: %s\n", string(outputDeleteStatefulsets))
		}

		cmdDeletePods := exec.Command("kubectl", "delete", "pod", "--all", "-n", namespace)
		outputDeletePods, err := cmdDeletePods.CombinedOutput()
		if err != nil {
			fmt.Printf("Failed to delete pods: %s\n", string(outputDeletePods))
		}
	})
})

func findPodName(allocationMap map[string]interface{}, targetPodName string) bool {
	// Iterate over the outer map
	for _, allocationData := range allocationMap {
		// Assert that the allocationData is of type map[string]interface{}
		allocation, ok := allocationData.(map[string]interface{})
		if !ok {
			continue // Skip if it's not the expected type
		}
		if podName, exists := allocation["podName"]; exists {
			if podNameStr, ok := podName.(string); ok && podNameStr == targetPodName {
				return false
			}
		}
	}
	return true
}

func isResourceReady(resource, label, namespace string) bool {
	var cmd = new(exec.Cmd)
	switch resource {
	case "pod", "pods", "po":
		cmd = exec.Command("kubectl", "wait", "--for=condition=ready", resource, "-l", label,
			"-n", namespace, "--timeout=2m",
		)
	case "service", "svc", "services":
		cmd = exec.Command("kubectl", "wait", "--for=jsonpath=spec.type=ClusterIP", resource, "-l", label,
			"-n", namespace, "--timeout=2m",
		)
	default:
		err := fmt.Errorf("unsupported resource : %s\n", resource)
		if err != nil {
			fmt.Printf("error printing %s", err)
		}
		return false
	}
	output, err := cmd.CombinedOutput()
	if err != nil {
		fmt.Printf("Waiting for the %s to be ready, err: %s\noutput: %s\nRetrying...\n", resource, err, output)
		return false
	}
	return true
}
