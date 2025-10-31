//go:build e2e && (vsphere || all_providers)
// +build e2e
// +build vsphere all_providers

package e2e

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"

	"github.com/aws/eks-anywhere/test/framework"
)

// ValidateNetworkUp validates that nodes have 2 different external IPs indicating both NICs are up
func ValidateNetworkUp(test *framework.ClusterE2ETest) {
	test.T.Log("Validating that nodes have 2 different external IPs (both NICs are up)")

	// Get all nodes
	nodes, err := getAllNodes(test)
	if err != nil {
		test.T.Fatalf("Failed to get nodes: %v", err)
	}

	for _, node := range nodes {
		test.T.Logf("Validating network interfaces for node: %s", node.Name)

		// Extract external IPs from node status
		externalIPs := getExternalIPsFromNode(node)

		if len(externalIPs) < 2 {
			test.T.Fatalf("Node %s does not have 2 external IPs. Found %d IPs: %v",
				node.Name, len(externalIPs), externalIPs)
		}

		// Validate that the IPs are different
		if !areIPsDifferent(externalIPs) {
			test.T.Fatalf("Node %s has duplicate external IPs: %v", node.Name, externalIPs)
		}

		test.T.Logf("Node %s has %d different external IPs: %v ✓",
			node.Name, len(externalIPs), externalIPs)
	}

	test.T.Log("Network validation completed successfully - all nodes have multiple external IPs")
}

// ValidateNetworkUpWithJSONPath validates network using JSONPath queries similar to your example
func ValidateNetworkUpWithJSONPath(test *framework.ClusterE2ETest) {
	test.T.Log("Validating network using JSONPath queries")

	ctx := context.Background()

	// Get all nodes using JSONPath to extract external IPs
	jsonPath := "{range .items[*]}{.metadata.name}{':'}{range .status.addresses[?(@.type=='ExternalIP')]}{.address}{','}{end}{'\n'}{end}"

	output, err := test.KubectlClient.ExecuteCommand(ctx,
		"get", "nodes",
		"-o", fmt.Sprintf("jsonpath=%s", jsonPath),
		"--kubeconfig", test.KubeconfigFilePath())

	if err != nil {
		test.T.Fatalf("Failed to get node external IPs: %v", err)
	}

	// Parse the output
	lines := strings.Split(strings.TrimSpace(output.String()), "\n")
	for _, line := range lines {
		if line == "" {
			continue
		}

		parts := strings.Split(line, ":")
		if len(parts) != 2 {
			continue
		}

		nodeName := parts[0]
		ipsStr := strings.TrimSuffix(parts[1], ",")

		if ipsStr == "" {
			test.T.Fatalf("Node %s has no external IPs", nodeName)
		}

		ips := strings.Split(ipsStr, ",")
		// Remove empty strings
		var validIPs []string
		for _, ip := range ips {
			if strings.TrimSpace(ip) != "" {
				validIPs = append(validIPs, strings.TrimSpace(ip))
			}
		}

		if len(validIPs) < 2 {
			test.T.Fatalf("Node %s does not have 2 external IPs. Found %d IPs: %v",
				nodeName, len(validIPs), validIPs)
		}

		if !areIPsDifferent(validIPs) {
			test.T.Fatalf("Node %s has duplicate external IPs: %v", nodeName, validIPs)
		}

		test.T.Logf("Node %s has %d different external IPs: %v ✓",
			nodeName, len(validIPs), validIPs)
	}

	test.T.Log("JSONPath network validation completed successfully")
}

// ValidateNetworkUpWithWaitLoop validates network using WaitJSONPathLoop similar to your example
func ValidateNetworkUpWithWaitLoop(test *framework.ClusterE2ETest) {
	test.T.Log("Validating network using WaitJSONPathLoop")

	// First get all node names
	nodes, err := getAllNodes(test)
	if err != nil {
		test.T.Fatalf("Failed to get nodes: %v", err)
	}

	for _, node := range nodes {
		test.T.Logf("Waiting for node %s to have multiple external IPs", node.Name)

		// Use a custom validation function that checks if we have multiple IPs
		err = waitForMultipleExternalIPs(test, node.Name, "5m")
		if err != nil {
			test.T.Fatalf("Node %s failed to get multiple external IPs within timeout: %v", node.Name, err)
		}

		test.T.Logf("Node %s successfully has multiple external IPs ✓", node.Name)
	}

	test.T.Log("WaitLoop network validation completed successfully")
}

// Helper function to get external IPs from a node
func getExternalIPsFromNode(node corev1.Node) []string {
	var externalIPs []string
	for _, addr := range node.Status.Addresses {
		if addr.Type == corev1.NodeExternalIP {
			externalIPs = append(externalIPs, addr.Address)
		}
	}
	return externalIPs
}

// Helper function to check if IPs are different
func areIPsDifferent(ips []string) bool {
	if len(ips) < 2 {
		return false
	}

	seen := make(map[string]bool)
	for _, ip := range ips {
		if seen[ip] {
			return false // Found duplicate
		}
		seen[ip] = true
	}
	return true
}

// Helper function to wait for multiple external IPs using a custom approach
func waitForMultipleExternalIPs(test *framework.ClusterE2ETest, nodeName, timeout string) error {
	ctx := context.Background()

	// Parse timeout
	timeoutDuration, err := time.ParseDuration(timeout)
	if err != nil {
		return fmt.Errorf("invalid timeout format: %v", err)
	}

	deadline := time.Now().Add(timeoutDuration)

	for time.Now().Before(deadline) {
		// Get the specific node
		output, err := test.KubectlClient.ExecuteCommand(ctx,
			"get", "node", nodeName,
			"-o", "json",
			"--kubeconfig", test.KubeconfigFilePath())

		if err != nil {
			test.T.Logf("Failed to get node %s, retrying: %v", nodeName, err)
			time.Sleep(10 * time.Second)
			continue
		}

		// Parse the node JSON
		var node corev1.Node
		if err := json.Unmarshal(output.Bytes(), &node); err != nil {
			test.T.Logf("Failed to parse node JSON, retrying: %v", err)
			time.Sleep(10 * time.Second)
			continue
		}

		// Check external IPs
		externalIPs := getExternalIPsFromNode(node)
		if len(externalIPs) >= 2 && areIPsDifferent(externalIPs) {
			test.T.Logf("Node %s now has %d different external IPs: %v",
				nodeName, len(externalIPs), externalIPs)
			return nil
		}

		test.T.Logf("Node %s has %d external IPs, waiting for 2+ different IPs: %v",
			nodeName, len(externalIPs), externalIPs)
		time.Sleep(10 * time.Second)
	}

	return fmt.Errorf("timeout waiting for node %s to have multiple external IPs", nodeName)
}

// getAllNodes gets all nodes in the cluster using kubectl
func getAllNodes(test *framework.ClusterE2ETest) ([]corev1.Node, error) {
	params := []string{"get", "nodes", "-o", "json", "--kubeconfig", test.KubeconfigFilePath()}
	stdOut, err := test.KubectlClient.Execute(context.Background(), params...)
	if err != nil {
		return nil, fmt.Errorf("getting nodes: %v", err)
	}

	response := &corev1.NodeList{}
	err = json.Unmarshal(stdOut.Bytes(), response)
	if err != nil {
		return nil, fmt.Errorf("unmarshaling nodes: %v", err)
	}

	return response.Items, nil
}
